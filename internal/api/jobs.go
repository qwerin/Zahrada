package api

// Jobs (zakázky) — SPEC §4.7.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"gorm.io/gorm"

	"github.com/qwerin/zahrada/internal/auth"
	"github.com/qwerin/zahrada/internal/events"
	"github.com/qwerin/zahrada/internal/model"
)

var jobStatuses = []string{model.JobInquiry, model.JobQuoted, model.JobOrdered, model.JobInProgress,
	model.JobDone, model.JobInvoiced, model.JobCancelled}

// jobTransitions is the status graph; anything else → 409 invalid_transition.
var jobTransitions = map[string][]string{
	model.JobInquiry:    {model.JobQuoted, model.JobOrdered, model.JobCancelled},
	model.JobQuoted:     {model.JobInquiry, model.JobOrdered, model.JobCancelled},
	model.JobOrdered:    {model.JobInProgress, model.JobDone, model.JobCancelled},
	model.JobInProgress: {model.JobDone, model.JobCancelled},
	model.JobDone:       {model.JobInProgress, model.JobInvoiced},
	model.JobInvoiced:   {model.JobDone},
	model.JobCancelled:  {model.JobInquiry},
}

var jobStatusLabels = map[string]string{
	model.JobInquiry: "poptávka", model.JobQuoted: "nabídka odeslána", model.JobOrdered: "objednáno",
	model.JobInProgress: "v realizaci", model.JobDone: "dokončeno", model.JobInvoiced: "vyfakturováno",
	model.JobCancelled: "zrušeno",
}

type Job struct {
	ID            uint      `json:"id"`
	Number        string    `json:"number"`
	ClientID      uint      `json:"client_id"`
	ClientName    string    `json:"client_name"`
	SiteID        *uint     `json:"site_id,omitempty"`
	SiteName      string    `json:"site_name,omitempty"`
	Kind          string    `json:"kind" enum:"realization,maintenance,one_off"`
	Title         string    `json:"title"`
	Status        string    `json:"status" enum:"inquiry,quoted,ordered,in_progress,done,invoiced,cancelled"`
	PlannedFrom   string    `json:"planned_from,omitempty" format:"date"`
	PlannedTo     string    `json:"planned_to,omitempty" format:"date"`
	MonthlyFee    *int64    `json:"monthly_fee,omitempty" doc:"Maintenance flat fee per month, haléře"`
	Note          string    `json:"note"`
	ContractorIDs []uint    `json:"contractor_ids" nullable:"false" doc:"Assigned contractors"`
	NextStatuses  []string  `json:"next_statuses" nullable:"false" doc:"Allowed status changes"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type JobCreate struct {
	ClientID      uint   `json:"client_id"`
	SiteID        *uint  `json:"site_id,omitempty" doc:"A garden of the client"`
	Kind          string `json:"kind" enum:"realization,maintenance,one_off"`
	Title         string `json:"title" minLength:"1" maxLength:"300"`
	Status        string `json:"status,omitempty" enum:"inquiry,quoted,ordered,in_progress" default:"inquiry"`
	PlannedFrom   string `json:"planned_from,omitempty" format:"date"`
	PlannedTo     string `json:"planned_to,omitempty" format:"date"`
	MonthlyFee    *int64 `json:"monthly_fee,omitempty" minimum:"0"`
	Note          string `json:"note,omitempty" maxLength:"10000"`
	ContractorIDs []uint `json:"contractor_ids,omitempty"`
}

type JobPatch struct {
	ClientID      *uint   `json:"client_id,omitempty"`
	SiteID        *uint   `json:"site_id,omitempty" doc:"0 removes the garden"`
	Kind          *string `json:"kind,omitempty" enum:"realization,maintenance,one_off"`
	Title         *string `json:"title,omitempty" minLength:"1" maxLength:"300"`
	PlannedFrom   *string `json:"planned_from,omitempty" doc:"YYYY-MM-DD, empty clears"`
	PlannedTo     *string `json:"planned_to,omitempty" doc:"YYYY-MM-DD, empty clears"`
	MonthlyFee    *int64  `json:"monthly_fee,omitempty" minimum:"0" doc:"0 clears"`
	Note          *string `json:"note,omitempty" maxLength:"10000"`
	ContractorIDs *[]uint `json:"contractor_ids,omitempty" doc:"Replaces the assigned contractors"`
}

type JobStatusChange struct {
	Status string `json:"status" enum:"inquiry,quoted,ordered,in_progress,done,invoiced,cancelled"`
}

func (s *server) registerJobs(g huma.API) {
	huma.Get(g, "/jobs", s.listJobs)
	huma.Post(g, "/jobs", s.createJob, status(http.StatusCreated), auth.ForEditors)
	huma.Get(g, "/jobs/{id}", s.getJob)
	huma.Patch(g, "/jobs/{id}", s.patchJob, auth.ForEditors)
	huma.Post(g, "/jobs/{id}/status", s.changeJobStatus, auth.ForEditors)
	huma.Delete(g, "/jobs/{id}", s.deleteJob, status(http.StatusNoContent), auth.ForEditors)
}

func toJob(m *model.Job) Job {
	next := jobTransitions[m.Status]
	if next == nil {
		next = []string{}
	}
	return Job{ID: m.ID, Number: m.Number, ClientID: m.ClientID, SiteID: m.SiteID, Kind: m.Kind, Title: m.Title,
		Status: m.Status, PlannedFrom: m.PlannedFrom, PlannedTo: m.PlannedTo, MonthlyFee: m.MonthlyFee, Note: m.Note,
		ContractorIDs: []uint{}, NextStatuses: next, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt}
}

// fillJobs adds client/site names and assigned contractors (a few queries per page).
func (s *server) fillJobs(ctx context.Context, db *gorm.DB, items []Job) error {
	var clientIDs, siteIDs, jobIDs []uint
	for _, j := range items {
		clientIDs = append(clientIDs, j.ClientID)
		jobIDs = append(jobIDs, j.ID)
		if j.SiteID != nil {
			siteIDs = append(siteIDs, *j.SiteID)
		}
	}
	clients := s.namesOf(ctx, db, &model.Client{}, clientIDs)
	sites := s.namesOf(ctx, db, &model.Site{}, siteIDs)
	var links []model.JobContractor
	if len(jobIDs) > 0 {
		if err := db.Scopes(inAccount(ctx)).Where("job_id IN ?", jobIDs).Order("contractor_id").Find(&links).Error; err != nil {
			return dbErr(err, "job contractor")
		}
	}
	for i := range items {
		items[i].ClientName = clients[items[i].ClientID]
		if items[i].SiteID != nil {
			items[i].SiteName = sites[*items[i].SiteID]
		}
		for _, l := range links {
			if l.JobID == items[i].ID {
				items[i].ContractorIDs = append(items[i].ContractorIDs, l.ContractorID)
			}
		}
	}
	return nil
}

func (s *server) jobOut(ctx context.Context, db *gorm.DB, m *model.Job) (Job, error) {
	out := []Job{toJob(m)}
	err := s.fillJobs(ctx, db, out)
	return out[0], err
}

func (s *server) listJobs(ctx context.Context, in *struct {
	PageParams
	Status       string `query:"status" doc:"Comma-separated statuses; \"open\" = everything not done, invoiced or cancelled"`
	Kind         string `query:"kind" enum:"realization,maintenance,one_off,"`
	ClientID     uint   `query:"client_id"`
	SiteID       uint   `query:"site_id"`
	ContractorID uint   `query:"contractor_id"`
	Query        string `query:"query" maxLength:"100"`
}) (*Out[ListResponse[Job]], error) {
	q := searchFilter(s.scoped(ctx).Model(&model.Job{}), in.Query).Order("id DESC")
	if in.Status != "" {
		var st []string
		for _, v := range splitStatuses(in.Status) {
			if v == "open" {
				st = append(st, model.JobInquiry, model.JobQuoted, model.JobOrdered, model.JobInProgress)
			} else if slices.Contains(jobStatuses, v) {
				st = append(st, v)
			} else {
				return nil, invalid("status", "unknown status "+v)
			}
		}
		q = q.Where("status IN ?", st)
	}
	if in.Kind != "" {
		q = q.Where("kind = ?", in.Kind)
	}
	if in.ClientID != 0 {
		q = q.Where("client_id = ?", in.ClientID)
	}
	if in.SiteID != 0 {
		q = q.Where("site_id = ?", in.SiteID)
	}
	if in.ContractorID != 0 {
		q = q.Where("id IN (?)", s.db.Model(&model.JobContractor{}).Select("job_id").Where("contractor_id = ?", in.ContractorID))
	}
	out, err := paginate(q, in.PageParams, toJob)
	if err != nil {
		return nil, err
	}
	return out, s.fillJobs(ctx, s.db.WithContext(ctx), out.Body.Items)
}

func (s *server) loadJob(ctx context.Context, db *gorm.DB, id uint) (*model.Job, error) {
	var m model.Job
	if err := db.Scopes(inAccount(ctx)).First(&m, id).Error; err != nil {
		return nil, dbErr(err, "job")
	}
	return &m, nil
}

func (s *server) getJob(ctx context.Context, in *idPath) (*Out[Job], error) {
	db := s.db.WithContext(ctx)
	m, err := s.loadJob(ctx, db, in.ID)
	if err != nil {
		return nil, err
	}
	out, err := s.jobOut(ctx, db, m)
	return &Out[Job]{Body: out}, err
}

// accountLocation is the time zone of the current account.
func (s *server) accountLocation(ctx context.Context) *time.Location {
	tz := defaultStr(auth.AccountFrom(ctx).TimeZone, defaultStr(s.cfg.TimeZone, "Europe/Prague"))
	if loc, err := time.LoadLocation(tz); err == nil {
		return loc
	}
	return time.UTC
}

// nextJobNumber returns Z{YYYY}-{NNN} after the highest number of the year (inside tx).
func nextJobNumber(ctx context.Context, tx *gorm.DB, year int) (string, error) {
	prefix := fmt.Sprintf("Z%d-", year)
	var nums []string
	if err := tx.Model(&model.Job{}).Scopes(inAccount(ctx)).Where("number LIKE ?", prefix+"%").Pluck("number", &nums).Error; err != nil {
		return "", dbErr(err, "job")
	}
	max := 0
	for _, n := range nums {
		var v int
		if _, err := fmt.Sscanf(strings.TrimPrefix(n, prefix), "%d", &v); err == nil && v > max {
			max = v
		}
	}
	return fmt.Sprintf("%s%03d", prefix, max+1), nil
}

// checkJobRefs validates client, site (must belong to the client) and dates.
func (s *server) checkJobRefs(ctx context.Context, tx *gorm.DB, m *model.Job) error {
	if _, err := s.loadClient(ctx, tx, m.ClientID); err != nil {
		return invalid("client_id", "client not found")
	}
	if m.SiteID != nil {
		site, err := s.loadSite(ctx, tx, *m.SiteID)
		if err != nil || site.ClientID != m.ClientID {
			return invalid("site_id", "garden not found for this client")
		}
	}
	for field, v := range map[string]string{"planned_from": m.PlannedFrom, "planned_to": m.PlannedTo} {
		if v != "" && !validDate(v) {
			return invalid(field, "use YYYY-MM-DD")
		}
	}
	if m.PlannedFrom != "" && m.PlannedTo != "" && m.PlannedTo < m.PlannedFrom {
		return invalid("planned_to", "must not be before planned_from")
	}
	return nil
}

// setJobContractors replaces the assignment of a job (inside tx).
func (s *server) setJobContractors(ctx context.Context, tx *gorm.DB, jobID uint, ids []uint) error {
	ids = slices.Compact(slices.Sorted(slices.Values(ids)))
	if len(ids) > 0 {
		var n int64
		if err := tx.Model(&model.Contractor{}).Scopes(inAccount(ctx)).Where("id IN ?", ids).Count(&n).Error; err != nil {
			return dbErr(err, "contractor")
		}
		if int(n) != len(ids) {
			return invalid("contractor_ids", "contractor not found")
		}
	}
	if err := tx.Where("job_id = ?", jobID).Delete(&model.JobContractor{}).Error; err != nil {
		return dbErr(err, "job contractor")
	}
	acc := auth.AccountFrom(ctx).ID
	for _, id := range ids {
		if err := tx.Create(&model.JobContractor{JobID: jobID, ContractorID: id, AccountID: acc}).Error; err != nil {
			return dbErr(err, "job contractor")
		}
	}
	return nil
}

func (s *server) createJob(ctx context.Context, in *struct{ Body JobCreate }) (*Out[Job], error) {
	b := in.Body
	m := model.Job{AccountID: auth.AccountFrom(ctx).ID, ClientID: b.ClientID, SiteID: b.SiteID, Kind: b.Kind,
		Title: strings.TrimSpace(b.Title), Status: defaultStr(b.Status, model.JobInquiry), PlannedFrom: b.PlannedFrom,
		PlannedTo: b.PlannedTo, MonthlyFee: b.MonthlyFee, Note: b.Note}
	if m.Title == "" {
		return nil, invalid("title", "title must not be empty")
	}
	year := s.deps.Now().In(s.accountLocation(ctx)).Year()
	var out Job
	var err error
	for attempt := 0; attempt < 3; attempt++ { // a concurrent create may take the same number
		err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := s.checkJobRefs(ctx, tx, &m); err != nil {
				return err
			}
			num, err := nextJobNumber(ctx, tx, year)
			if err != nil {
				return err
			}
			m.ID, m.Number = 0, num
			if err := tx.Create(&m).Error; err != nil {
				return err
			}
			if err := s.setJobContractors(ctx, tx, m.ID, b.ContractorIDs); err != nil {
				return err
			}
			if err := recordSimple(ctx, tx, events.JobCreated, events.SubjectJob, m.ID,
				"Zakázka "+m.Number+" "+m.Title+" vytvořena", map[string]any{"number": m.Number}); err != nil {
				return err
			}
			out, err = s.jobOut(ctx, tx, &m)
			return err
		})
		if !errors.Is(err, gorm.ErrDuplicatedKey) {
			break
		}
	}
	if err != nil {
		var se huma.StatusError
		if errors.As(err, &se) {
			return nil, err
		}
		return nil, dbErr(err, "job")
	}
	return &Out[Job]{Body: out}, nil
}

func (s *server) patchJob(ctx context.Context, in *struct {
	ID   uint `path:"id"`
	Body JobPatch
}) (*Out[Job], error) {
	var out Job
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		m, err := s.loadJob(ctx, tx, in.ID)
		if err != nil {
			return err
		}
		p := in.Body
		apply(&m.ClientID, p.ClientID)
		if p.SiteID != nil {
			if *p.SiteID == 0 {
				m.SiteID = nil
			} else {
				m.SiteID = p.SiteID
			}
		}
		apply(&m.Kind, p.Kind)
		apply(&m.Title, trimmed(p.Title))
		apply(&m.PlannedFrom, p.PlannedFrom)
		apply(&m.PlannedTo, p.PlannedTo)
		if p.MonthlyFee != nil {
			if *p.MonthlyFee == 0 {
				m.MonthlyFee = nil
			} else {
				m.MonthlyFee = p.MonthlyFee
			}
		}
		apply(&m.Note, p.Note)
		if m.Title == "" {
			return invalid("title", "title must not be empty")
		}
		if err := s.checkJobRefs(ctx, tx, m); err != nil {
			return err
		}
		if err := tx.Save(m).Error; err != nil {
			return dbErr(err, "job")
		}
		if p.ContractorIDs != nil {
			if err := s.setJobContractors(ctx, tx, m.ID, *p.ContractorIDs); err != nil {
				return err
			}
		}
		if err := recordSimple(ctx, tx, events.JobUpdated, events.SubjectJob, m.ID, "Zakázka "+m.Number+" upravena", nil); err != nil {
			return err
		}
		out, err = s.jobOut(ctx, tx, m)
		return err
	})
	return &Out[Job]{Body: out}, err
}

// setJobStatus validates and records a status change (inside tx).
func setJobStatus(ctx context.Context, tx *gorm.DB, m *model.Job, to string) error {
	if m.Status == to {
		return nil
	}
	if !slices.Contains(jobTransitions[m.Status], to) {
		return conflict(CodeInvalidTransition, fmt.Sprintf("a job cannot go from %s to %s", m.Status, to))
	}
	from := m.Status
	m.Status = to
	if err := tx.Model(m).Update("status", to).Error; err != nil {
		return dbErr(err, "job")
	}
	return recordSimple(ctx, tx, events.JobStatusChanged, events.SubjectJob, m.ID,
		fmt.Sprintf("Zakázka %s: %s → %s", m.Number, jobStatusLabels[from], jobStatusLabels[to]),
		map[string]any{"from": from, "to": to})
}

func (s *server) changeJobStatus(ctx context.Context, in *struct {
	ID   uint `path:"id"`
	Body JobStatusChange
}) (*Out[Job], error) {
	var out Job
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		m, err := s.loadJob(ctx, tx, in.ID)
		if err != nil {
			return err
		}
		if err := setJobStatus(ctx, tx, m, in.Body.Status); err != nil {
			return err
		}
		out, err = s.jobOut(ctx, tx, m)
		return err
	})
	return &Out[Job]{Body: out}, err
}

func (s *server) deleteJob(ctx context.Context, in *idPath) (*NoContent, error) {
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		m, err := s.loadJob(ctx, tx, in.ID)
		if err != nil {
			return err
		}
		var n int64
		if err := tx.Model(&model.AttendanceEntry{}).Scopes(inAccount(ctx)).Where("job_id = ?", m.ID).Count(&n).Error; err != nil {
			return dbErr(err, "attendance")
		}
		if n > 0 {
			return conflict(CodeHasAttendance, "the job has attendance records; cancel it instead")
		}
		if err := tx.Where("job_id = ?", m.ID).Delete(&model.JobContractor{}).Error; err != nil {
			return dbErr(err, "job contractor")
		}
		if err := tx.Delete(m).Error; err != nil {
			return dbErr(err, "job")
		}
		return recordSimple(ctx, tx, events.JobDeleted, events.SubjectJob, m.ID, "Zakázka "+m.Number+" smazána", nil)
	})
	if err != nil {
		return nil, err
	}
	return &NoContent{}, nil
}
