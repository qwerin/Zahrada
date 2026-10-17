package api

import (
	"context"
	"fmt"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"github.com/qwerin/zahrada/internal/auth"
	"github.com/qwerin/zahrada/internal/model"
	"github.com/qwerin/zahrada/internal/search"
)

// SearchHit is one global search result.
type SearchHit struct {
	Type     string `json:"type" enum:"job,client,site,contractor"`
	ID       uint   `json:"id"`
	Title    string `json:"title" doc:"Number / name"`
	Subtitle string `json:"subtitle" doc:"Client, address, IČO …"`
	Status   string `json:"status,omitempty"`
	URLHint  string `json:"url_hint" doc:"SPA path of the detail, e.g. /a/{slug}/jobs/12"`
}

// SearchResults are the hits grouped by type (each at most limit).
type SearchResults struct {
	Jobs        []SearchHit `json:"jobs" nullable:"false"`
	Clients     []SearchHit `json:"clients" nullable:"false"`
	Sites       []SearchHit `json:"sites" nullable:"false"`
	Contractors []SearchHit `json:"contractors" nullable:"false"`
}

func (s *server) registerSearch(g huma.API) {
	huma.Get(g, "/search", s.globalSearch, func(o *huma.Operation) {
		o.Description = "Case- and diacritics-insensitive search in jobs (number, title), clients (name, IČO, e-mail, phone, address), " +
			"gardens (name, address) and contractors (name, IČO, e-mail, phone)."
	})
}

func (s *server) globalSearch(ctx context.Context, in *struct {
	Q     string `query:"q" minLength:"1" maxLength:"100" required:"true"`
	Limit int    `query:"limit" minimum:"1" maximum:"20" default:"5"`
}) (*Out[SearchResults], error) {
	res := SearchResults{Jobs: []SearchHit{}, Clients: []SearchHit{}, Sites: []SearchHit{}, Contractors: []SearchHit{}}
	if search.Fold(in.Q) == "" {
		return &Out[SearchResults]{Body: res}, nil
	}
	pattern := search.LikePattern(in.Q)
	base := "/a/" + auth.AccountFrom(ctx).Slug
	match := func(v any, order string, dst any) error {
		return s.scoped(ctx).Model(v).Where(`search_text LIKE ? ESCAPE '\'`, pattern).Order(order).Limit(in.Limit).Find(dst).Error
	}

	var jobs []model.Job
	if err := match(&model.Job{}, "id DESC", &jobs); err != nil {
		return nil, dbErr(err, "job")
	}
	for _, m := range jobs {
		res.Jobs = append(res.Jobs, SearchHit{Type: "job", ID: m.ID, Title: m.Number + " " + m.Title, Status: m.Status,
			URLHint: fmt.Sprintf("%s/jobs/%d", base, m.ID)})
	}
	var clients []model.Client
	if err := match(&model.Client{}, "name, id", &clients); err != nil {
		return nil, dbErr(err, "client")
	}
	for _, m := range clients {
		res.Clients = append(res.Clients, SearchHit{Type: "client", ID: m.ID, Title: m.Name,
			Subtitle: joinNonEmpty(m.City, m.Phone, m.Email), URLHint: fmt.Sprintf("%s/clients/%d", base, m.ID)})
	}
	var sites []model.Site
	if err := match(&model.Site{}, "name, id", &sites); err != nil {
		return nil, dbErr(err, "site")
	}
	for _, m := range sites {
		res.Sites = append(res.Sites, SearchHit{Type: "site", ID: m.ID, Title: m.Name,
			Subtitle: joinNonEmpty(m.Street, m.City), URLHint: fmt.Sprintf("%s/clients/%d", base, m.ClientID)})
	}
	if auth.RequireRole(ctx, auth.RolesEditors...) == nil { // rates are not for the accountant
		var cs []model.Contractor
		if err := match(&model.Contractor{}, "name, id", &cs); err != nil {
			return nil, dbErr(err, "contractor")
		}
		for _, m := range cs {
			res.Contractors = append(res.Contractors, SearchHit{Type: "contractor", ID: m.ID, Title: m.Name,
				Subtitle: joinNonEmpty(m.Phone, m.Email), URLHint: fmt.Sprintf("%s/contractors/%d", base, m.ID)})
		}
	}
	return &Out[SearchResults]{Body: res}, nil
}

func joinNonEmpty(parts ...string) string {
	var out []string
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, " · ")
}
