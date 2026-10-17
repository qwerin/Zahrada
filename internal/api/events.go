package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"gorm.io/gorm"

	"github.com/qwerin/zahrada/internal/auth"
	"github.com/qwerin/zahrada/internal/events"
	"github.com/qwerin/zahrada/internal/model"
)

// ---- recording (used at every domain mutation point) ----

type clockKey struct{}

// withClock makes now the clock of record() and the todo sync for ctx.
// api.New installs it for every request and systemContext for jobs.
func withClock(ctx context.Context, now func() time.Time) context.Context {
	return context.WithValue(ctx, clockKey{}, now)
}

// nowFrom is the clock of ctx (time.Now when none was installed).
func nowFrom(ctx context.Context) time.Time {
	if f, ok := ctx.Value(clockKey{}).(func() time.Time); ok {
		return f()
	}
	return time.Now()
}

// record writes event e of the current account (user from ctx, nil in jobs
// and public endpoints) inside tx — the transaction of the change — and
// refreshes the automatic todos of e's subject. Call it at each mutation
// point, usually through the typed helpers below:
//
//	if err := recordSimple(ctx, tx, events.JobCreated, events.SubjectJob, m.ID, "…", nil); err != nil {
//		return err
//	}
func record(ctx context.Context, tx *gorm.DB, e events.Event) error {
	acc := auth.AccountFrom(ctx)
	meta := events.Meta{AccountID: acc.ID, AccountSlug: acc.Slug, At: nowFrom(ctx)}
	if u := auth.UserFrom(ctx); u != nil {
		id := u.ID
		meta.UserID = &id
	}
	if _, err := events.Record(tx, meta, e); err != nil {
		return huma.Error500InternalServerError("cannot record event", err)
	}
	return syncTodos(ctx, tx, e.SubjectType, e.SubjectID)
}

// recordSimple records a created/updated/deleted event of a record with a
// Czech sentence and its key fields.
func recordSimple(ctx context.Context, tx *gorm.DB, name, subjectType string, id uint, text string, data map[string]any) error {
	if data == nil {
		data = map[string]any{}
	}
	data["id"] = id
	return record(ctx, tx, events.Event{Name: name, SubjectType: subjectType, SubjectID: id, Text: text, Data: data})
}

// czDate formats "YYYY-MM-DD" as "15. 3. 2026".
func czDate(iso string) string {
	t, err := time.Parse(time.DateOnly, iso)
	if err != nil {
		return iso
	}
	return fmt.Sprintf("%d. %d. %d", t.Day(), int(t.Month()), t.Year())
}

// ---- API ----

// Event is one activity log entry.
type Event struct {
	ID          uint           `json:"id"`
	Name        string         `json:"name" doc:"e.g. job.created (see GET /events/catalog)"`
	SubjectType string         `json:"subject_type" doc:"client, site, contractor, job, attendance, webhook, account"`
	SubjectID   uint           `json:"subject_id"`
	Text        string         `json:"text" doc:"Czech description"`
	Data        map[string]any `json:"data"`
	UserID      *uint          `json:"user_id,omitempty" doc:"Author; missing for the scheduler and public link"`
	UserName    string         `json:"user_name,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
}

// EventCatalogEntry is a known event name.
type EventCatalogEntry = events.CatalogEntry

func toEvent(m *model.Event) Event {
	data := m.Data
	if data == nil {
		data = map[string]any{}
	}
	return Event{ID: m.ID, Name: m.Name, SubjectType: m.SubjectType, SubjectID: m.SubjectID, Text: m.Text,
		Data: data, UserID: m.UserID, CreatedAt: m.CreatedAt}
}

func (s *server) registerEvents(g huma.API) {
	huma.Get(g, "/events", s.listEvents)
	huma.Get(g, "/events/catalog", s.eventCatalog)
}

func (s *server) listEvents(ctx context.Context, in *struct {
	PageParams
	Name        string `query:"name" doc:"Exact name (job.created) or prefix (job, job.*)"`
	SubjectType string `query:"subject_type"`
	SubjectID   uint   `query:"subject_id" doc:"With subject_type: the timeline of one record"`
	Since       string `query:"since" doc:"RFC 3339 instant or YYYY-MM-DD (inclusive)"`
}) (*Out[ListResponse[Event]], error) {
	q := s.scoped(ctx).Model(&model.Event{}).Order("created_at DESC, id DESC")
	if auth.RequireRole(ctx, auth.RolesManagers...) != nil {
		// webhooks are managers-only (also their events: older ones hold the URL)
		q = q.Where("subject_type <> ?", events.SubjectWebhook)
	}
	if n := strings.TrimSuffix(strings.TrimSpace(in.Name), "*"); n != "" {
		if strings.Contains(n, ".") && !strings.HasSuffix(n, ".") {
			q = q.Where("name = ?", n)
		} else {
			q = q.Where("name LIKE ?", strings.TrimSuffix(n, ".")+".%")
		}
	}
	if in.SubjectType != "" {
		q = q.Where("subject_type = ?", in.SubjectType)
	}
	if in.SubjectID != 0 {
		if in.SubjectType == "" {
			return nil, huma.NewError(http.StatusUnprocessableEntity, "validation failed",
				&huma.ErrorDetail{Location: "query.subject_type", Message: "subject_id requires subject_type"})
		}
		q = q.Where("subject_id = ?", in.SubjectID)
	}
	if in.Since != "" {
		t, err := time.Parse(time.RFC3339, in.Since)
		if err != nil {
			if t, err = time.Parse("2006-01-02", in.Since); err != nil {
				return nil, huma.NewError(http.StatusUnprocessableEntity, "validation failed",
					&huma.ErrorDetail{Location: "query.since", Message: "use RFC 3339 or YYYY-MM-DD", Value: in.Since})
			}
		}
		q = q.Where("created_at >= ?", t)
	}
	out, err := paginate(q, in.PageParams, toEvent)
	if err != nil {
		return nil, err
	}
	s.fillEventUsers(ctx, out.Body.Items)
	return out, nil
}

// fillEventUsers adds user names (one query per page).
func (s *server) fillEventUsers(ctx context.Context, items []Event) {
	ids := []uint{}
	for _, e := range items {
		if e.UserID != nil {
			ids = append(ids, *e.UserID)
		}
	}
	if len(ids) == 0 {
		return
	}
	var users []model.User
	if err := s.db.WithContext(ctx).Select("id", "name", "email").Where("id IN ?", ids).Find(&users).Error; err != nil {
		return
	}
	names := map[uint]string{}
	for _, u := range users {
		names[u.ID] = defaultStr(u.Name, u.Email)
	}
	for i := range items {
		if items[i].UserID != nil {
			items[i].UserName = names[*items[i].UserID]
		}
	}
}

func (s *server) eventCatalog(ctx context.Context, _ *struct{}) (*Out[[]EventCatalogEntry], error) {
	return &Out[[]EventCatalogEntry]{Body: events.Catalog}, nil
}
