package api

// Clients, their gardens (sites) and contractors — SPEC §4.2–4.4.

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
	"github.com/qwerin/zahrada/internal/search"
)

// ---- DTOs ----

type Client struct {
	ID             uint      `json:"id"`
	Name           string    `json:"name"`
	Kind           string    `json:"kind" enum:"person,company"`
	RegistrationNo string    `json:"registration_no"`
	VatNo          string    `json:"vat_no"`
	Email          string    `json:"email"`
	Phone          string    `json:"phone"`
	Street         string    `json:"street"`
	City           string    `json:"city"`
	Zip            string    `json:"zip"`
	Note           string    `json:"note"`
	Archived       bool      `json:"archived"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type ClientDetail struct {
	Client
	Sites []Site `json:"sites" nullable:"false"`
}

type ClientCreate struct {
	Name           string `json:"name" minLength:"1" maxLength:"200"`
	Kind           string `json:"kind,omitempty" enum:"person,company" default:"person"`
	RegistrationNo string `json:"registration_no,omitempty" maxLength:"20"`
	VatNo          string `json:"vat_no,omitempty" maxLength:"20"`
	Email          string `json:"email,omitempty" maxLength:"254"`
	Phone          string `json:"phone,omitempty" maxLength:"50"`
	Street         string `json:"street,omitempty" maxLength:"200"`
	City           string `json:"city,omitempty" maxLength:"100"`
	Zip            string `json:"zip,omitempty" maxLength:"20"`
	Note           string `json:"note,omitempty" maxLength:"5000"`
}

type ClientPatch struct {
	Name           *string `json:"name,omitempty" minLength:"1" maxLength:"200"`
	Kind           *string `json:"kind,omitempty" enum:"person,company"`
	RegistrationNo *string `json:"registration_no,omitempty" maxLength:"20"`
	VatNo          *string `json:"vat_no,omitempty" maxLength:"20"`
	Email          *string `json:"email,omitempty" maxLength:"254"`
	Phone          *string `json:"phone,omitempty" maxLength:"50"`
	Street         *string `json:"street,omitempty" maxLength:"200"`
	City           *string `json:"city,omitempty" maxLength:"100"`
	Zip            *string `json:"zip,omitempty" maxLength:"20"`
	Note           *string `json:"note,omitempty" maxLength:"5000"`
	Archived       *bool   `json:"archived,omitempty"`
}

func toClient(m *model.Client) Client {
	return Client{ID: m.ID, Name: m.Name, Kind: m.Kind, RegistrationNo: m.RegistrationNo, VatNo: m.VatNo,
		Email: m.Email, Phone: m.Phone, Street: m.Street, City: m.City, Zip: m.Zip, Note: m.Note,
		Archived: m.Archived, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt}
}

type Site struct {
	ID         uint      `json:"id"`
	ClientID   uint      `json:"client_id"`
	ClientName string    `json:"client_name,omitempty"`
	Name       string    `json:"name"`
	Street     string    `json:"street"`
	City       string    `json:"city"`
	Zip        string    `json:"zip"`
	Lat        *float64  `json:"lat,omitempty"`
	Lng        *float64  `json:"lng,omitempty"`
	AreaM2     *int64    `json:"area_m2,omitempty"`
	LawnM2     *int64    `json:"lawn_m2,omitempty"`
	HedgeBM    *int64    `json:"hedge_bm,omitempty"`
	AccessNote string    `json:"access_note" doc:"Keys, gate code, dog … (contractors see it)"`
	Note       string    `json:"note"`
	Archived   bool      `json:"archived"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type SiteCreate struct {
	ClientID   uint     `json:"client_id"`
	Name       string   `json:"name" minLength:"1" maxLength:"200"`
	Street     string   `json:"street,omitempty" maxLength:"200"`
	City       string   `json:"city,omitempty" maxLength:"100"`
	Zip        string   `json:"zip,omitempty" maxLength:"20"`
	Lat        *float64 `json:"lat,omitempty" minimum:"-90" maximum:"90"`
	Lng        *float64 `json:"lng,omitempty" minimum:"-180" maximum:"180"`
	AreaM2     *int64   `json:"area_m2,omitempty" minimum:"0"`
	LawnM2     *int64   `json:"lawn_m2,omitempty" minimum:"0"`
	HedgeBM    *int64   `json:"hedge_bm,omitempty" minimum:"0"`
	AccessNote string   `json:"access_note,omitempty" maxLength:"2000"`
	Note       string   `json:"note,omitempty" maxLength:"5000"`
}

type SitePatch struct {
	Name       *string  `json:"name,omitempty" minLength:"1" maxLength:"200"`
	Street     *string  `json:"street,omitempty" maxLength:"200"`
	City       *string  `json:"city,omitempty" maxLength:"100"`
	Zip        *string  `json:"zip,omitempty" maxLength:"20"`
	Lat        *float64 `json:"lat,omitempty" minimum:"-90" maximum:"90"`
	Lng        *float64 `json:"lng,omitempty" minimum:"-180" maximum:"180"`
	AreaM2     *int64   `json:"area_m2,omitempty" minimum:"0"`
	LawnM2     *int64   `json:"lawn_m2,omitempty" minimum:"0"`
	HedgeBM    *int64   `json:"hedge_bm,omitempty" minimum:"0"`
	AccessNote *string  `json:"access_note,omitempty" maxLength:"2000"`
	Note       *string  `json:"note,omitempty" maxLength:"5000"`
	Archived   *bool    `json:"archived,omitempty"`
}

func toSite(m *model.Site) Site {
	return Site{ID: m.ID, ClientID: m.ClientID, Name: m.Name, Street: m.Street, City: m.City, Zip: m.Zip,
		Lat: m.Lat, Lng: m.Lng, AreaM2: m.AreaM2, LawnM2: m.LawnM2, HedgeBM: m.HedgeBM,
		AccessNote: m.AccessNote, Note: m.Note, Archived: m.Archived, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt}
}

type Contractor struct {
	ID             uint      `json:"id"`
	Name           string    `json:"name"`
	Kind           string    `json:"kind" enum:"person,company"`
	RegistrationNo string    `json:"registration_no"`
	Phone          string    `json:"phone"`
	Email          string    `json:"email"`
	HourlyRate     int64     `json:"hourly_rate" doc:"Haléře per hour"`
	KmRate         int64     `json:"km_rate" doc:"Haléře per km"`
	Color          string    `json:"color"`
	Active         bool      `json:"active"`
	UserID         *uint     `json:"user_id,omitempty" doc:"Linked login (role contractor)"`
	UserEmail      string    `json:"user_email,omitempty"`
	Login          string    `json:"login" enum:"email,link,none" doc:"How the contractor signs in: e-mail account, one-time links, or not at all"`
	Note           string    `json:"note"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type ContractorCreate struct {
	Name           string `json:"name" minLength:"1" maxLength:"200"`
	Kind           string `json:"kind,omitempty" enum:"person,company" default:"person"`
	RegistrationNo string `json:"registration_no,omitempty" maxLength:"20"`
	Phone          string `json:"phone,omitempty" maxLength:"50"`
	Email          string `json:"email,omitempty" maxLength:"254"`
	HourlyRate     *int64 `json:"hourly_rate,omitempty" minimum:"0" doc:"Omitted = account default"`
	KmRate         *int64 `json:"km_rate,omitempty" minimum:"0" doc:"Omitted = account default"`
	Color          string `json:"color,omitempty" pattern:"^(#[0-9A-Fa-f]{6})?$"`
	Note           string `json:"note,omitempty" maxLength:"5000"`
}

type ContractorPatch struct {
	Name           *string `json:"name,omitempty" minLength:"1" maxLength:"200"`
	Kind           *string `json:"kind,omitempty" enum:"person,company"`
	RegistrationNo *string `json:"registration_no,omitempty" maxLength:"20"`
	Phone          *string `json:"phone,omitempty" maxLength:"50"`
	Email          *string `json:"email,omitempty" maxLength:"254"`
	HourlyRate     *int64  `json:"hourly_rate,omitempty" minimum:"0" doc:"Applies to new attendance only"`
	KmRate         *int64  `json:"km_rate,omitempty" minimum:"0"`
	Color          *string `json:"color,omitempty" pattern:"^(#[0-9A-Fa-f]{6})?$"`
	Active         *bool   `json:"active,omitempty"`
	Note           *string `json:"note,omitempty" maxLength:"5000"`
}

type ContractorInvite struct {
	Email string `json:"email,omitempty" maxLength:"254" doc:"Omitted = the contractor's e-mail"`
}

func toContractor(m *model.Contractor) Contractor {
	return Contractor{ID: m.ID, Name: m.Name, Kind: m.Kind, RegistrationNo: m.RegistrationNo, Phone: m.Phone,
		Email: m.Email, HourlyRate: m.HourlyRate, KmRate: m.KmRate, Color: m.Color, Active: m.Active,
		UserID: m.UserID, Note: m.Note, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt}
}

// ---- routes ----

func (s *server) registerDirectory(g huma.API) {
	huma.Get(g, "/clients", s.listClients)
	huma.Post(g, "/clients", s.createClient, status(http.StatusCreated), auth.ForEditors)
	huma.Get(g, "/clients/{id}", s.getClient)
	huma.Patch(g, "/clients/{id}", s.patchClient, auth.ForEditors)
	huma.Delete(g, "/clients/{id}", s.deleteClient, status(http.StatusNoContent), auth.ForEditors)

	huma.Get(g, "/sites", s.listSites)
	huma.Post(g, "/sites", s.createSite, status(http.StatusCreated), auth.ForEditors)
	huma.Get(g, "/sites/{id}", s.getSite)
	huma.Patch(g, "/sites/{id}", s.patchSite, auth.ForEditors)
	huma.Delete(g, "/sites/{id}", s.deleteSite, status(http.StatusNoContent), auth.ForEditors)

	huma.Get(g, "/contractors", s.listContractors)
	huma.Post(g, "/contractors", s.createContractor, status(http.StatusCreated), auth.ForEditors)
	huma.Get(g, "/contractors/{id}", s.getContractor)
	huma.Patch(g, "/contractors/{id}", s.patchContractor, auth.ForEditors)
	huma.Delete(g, "/contractors/{id}", s.deleteContractor, status(http.StatusNoContent), auth.ForEditors)
	huma.Post(g, "/contractors/{id}/invite", s.inviteContractor, status(http.StatusCreated), auth.ForManagers)
}

type idPath struct {
	ID uint `path:"id"`
}

// searchFilter applies a diacritics-insensitive search on search_text.
func searchFilter(q *gorm.DB, query string) *gorm.DB {
	if search.Fold(query) == "" {
		return q
	}
	return q.Where(`search_text LIKE ? ESCAPE '\'`, search.LikePattern(query))
}

func trimmed(p *string) *string {
	if p == nil {
		return nil
	}
	v := strings.TrimSpace(*p)
	return &v
}

// ---- clients ----

func (s *server) listClients(ctx context.Context, in *struct {
	PageParams
	Query    string `query:"query" maxLength:"100"`
	Archived bool   `query:"archived" doc:"true = archived clients only"`
}) (*Out[ListResponse[Client]], error) {
	q := searchFilter(s.scoped(ctx).Model(&model.Client{}), in.Query).Where("archived = ?", in.Archived).Order("LOWER(name), id")
	return paginate(q, in.PageParams, toClient)
}

func (s *server) loadClient(ctx context.Context, db *gorm.DB, id uint) (*model.Client, error) {
	var m model.Client
	if err := db.Scopes(inAccount(ctx)).First(&m, id).Error; err != nil {
		return nil, dbErr(err, "client")
	}
	return &m, nil
}

func (s *server) clientDetail(ctx context.Context, db *gorm.DB, m *model.Client) (ClientDetail, error) {
	var sites []model.Site
	if err := db.Scopes(inAccount(ctx)).Where("client_id = ?", m.ID).Order("archived, LOWER(name), id").Find(&sites).Error; err != nil {
		return ClientDetail{}, dbErr(err, "site")
	}
	out := ClientDetail{Client: toClient(m), Sites: make([]Site, len(sites))}
	for i := range sites {
		out.Sites[i] = toSite(&sites[i])
	}
	return out, nil
}

func (s *server) getClient(ctx context.Context, in *idPath) (*Out[ClientDetail], error) {
	db := s.db.WithContext(ctx)
	m, err := s.loadClient(ctx, db, in.ID)
	if err != nil {
		return nil, err
	}
	d, err := s.clientDetail(ctx, db, m)
	return &Out[ClientDetail]{Body: d}, err
}

func (s *server) createClient(ctx context.Context, in *struct{ Body ClientCreate }) (*Out[ClientDetail], error) {
	b := in.Body
	m := model.Client{AccountID: auth.AccountFrom(ctx).ID, Name: strings.TrimSpace(b.Name), Kind: defaultStr(b.Kind, "person"),
		RegistrationNo: strings.TrimSpace(b.RegistrationNo), VatNo: strings.TrimSpace(b.VatNo), Email: strings.TrimSpace(b.Email),
		Phone: strings.TrimSpace(b.Phone), Street: b.Street, City: b.City, Zip: b.Zip, Note: b.Note}
	if m.Name == "" {
		return nil, invalid("name", "name must not be empty")
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&m).Error; err != nil {
			return dbErr(err, "client")
		}
		return recordSimple(ctx, tx, events.ClientCreated, events.SubjectClient, m.ID, "Klient "+m.Name+" vytvořen", map[string]any{"name": m.Name})
	})
	if err != nil {
		return nil, err
	}
	return &Out[ClientDetail]{Body: ClientDetail{Client: toClient(&m), Sites: []Site{}}}, nil
}

func (s *server) patchClient(ctx context.Context, in *struct {
	ID   uint `path:"id"`
	Body ClientPatch
}) (*Out[ClientDetail], error) {
	var out ClientDetail
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		m, err := s.loadClient(ctx, tx, in.ID)
		if err != nil {
			return err
		}
		p := in.Body
		apply(&m.Name, trimmed(p.Name))
		apply(&m.Kind, p.Kind)
		apply(&m.RegistrationNo, trimmed(p.RegistrationNo))
		apply(&m.VatNo, trimmed(p.VatNo))
		apply(&m.Email, trimmed(p.Email))
		apply(&m.Phone, trimmed(p.Phone))
		apply(&m.Street, p.Street)
		apply(&m.City, p.City)
		apply(&m.Zip, p.Zip)
		apply(&m.Note, p.Note)
		apply(&m.Archived, p.Archived)
		if m.Name == "" {
			return invalid("name", "name must not be empty")
		}
		if err := tx.Save(m).Error; err != nil {
			return dbErr(err, "client")
		}
		if err := recordSimple(ctx, tx, events.ClientUpdated, events.SubjectClient, m.ID, "Klient "+m.Name+" upraven", nil); err != nil {
			return err
		}
		out, err = s.clientDetail(ctx, tx, m)
		return err
	})
	return &Out[ClientDetail]{Body: out}, err
}

func (s *server) deleteClient(ctx context.Context, in *idPath) (*NoContent, error) {
	var files []string
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		m, err := s.loadClient(ctx, tx, in.ID)
		if err != nil {
			return err
		}
		var n int64
		if err := tx.Model(&model.Job{}).Scopes(inAccount(ctx)).Where("client_id = ?", m.ID).Count(&n).Error; err != nil {
			return dbErr(err, "job")
		}
		if n > 0 {
			return conflict(CodeHasJobs, "the client has jobs; archive it instead")
		}
		var siteIDs []uint
		if err := tx.Model(&model.Site{}).Scopes(inAccount(ctx)).Where("client_id = ?", m.ID).Pluck("id", &siteIDs).Error; err != nil {
			return dbErr(err, "site")
		}
		for _, id := range siteIDs {
			f, err := deleteOwnerAttachments(ctx, tx, model.OwnerSite, id)
			if err != nil {
				return err
			}
			files = append(files, f...)
		}
		if err := tx.Scopes(inAccount(ctx)).Where("client_id = ?", m.ID).Delete(&model.Site{}).Error; err != nil {
			return dbErr(err, "site")
		}
		f, err := deleteOwnerAttachments(ctx, tx, model.OwnerClient, m.ID)
		if err != nil {
			return err
		}
		files = append(files, f...)
		if err := tx.Delete(m).Error; err != nil {
			return dbErr(err, "client")
		}
		return recordSimple(ctx, tx, events.ClientDeleted, events.SubjectClient, m.ID, "Klient "+m.Name+" smazán", nil)
	})
	if err != nil {
		return nil, err
	}
	s.removeFiles(ctx, files)
	return &NoContent{}, nil
}

// ---- sites ----

func (s *server) listSites(ctx context.Context, in *struct {
	PageParams
	ClientID uint   `query:"client_id"`
	Query    string `query:"query" maxLength:"100"`
	Archived bool   `query:"archived"`
}) (*Out[ListResponse[Site]], error) {
	q := searchFilter(s.scoped(ctx).Model(&model.Site{}), in.Query).Where("archived = ?", in.Archived).Order("LOWER(name), id")
	if in.ClientID != 0 {
		q = q.Where("client_id = ?", in.ClientID)
	}
	out, err := paginate(q, in.PageParams, toSite)
	if err != nil {
		return nil, err
	}
	s.fillSiteClients(ctx, out.Body.Items)
	return out, nil
}

func (s *server) fillSiteClients(ctx context.Context, items []Site) {
	ids := make([]uint, 0, len(items))
	for _, it := range items {
		ids = append(ids, it.ClientID)
	}
	names := s.namesOf(ctx, s.db.WithContext(ctx), &model.Client{}, ids)
	for i := range items {
		items[i].ClientName = names[items[i].ClientID]
	}
}

// namesOf maps IDs of an account-scoped table (with a name column) to names.
// Pass the transaction when called inside one (SQLite has a single connection).
func (s *server) namesOf(ctx context.Context, db *gorm.DB, m any, ids []uint) map[uint]string {
	out := map[uint]string{}
	if len(ids) == 0 {
		return out
	}
	var rows []struct {
		ID   uint
		Name string
	}
	if err := db.Scopes(inAccount(ctx)).Model(m).Select("id", "name").Where("id IN ?", ids).Find(&rows).Error; err == nil {
		for _, r := range rows {
			out[r.ID] = r.Name
		}
	}
	return out
}

func (s *server) loadSite(ctx context.Context, db *gorm.DB, id uint) (*model.Site, error) {
	var m model.Site
	if err := db.Scopes(inAccount(ctx)).First(&m, id).Error; err != nil {
		return nil, dbErr(err, "site")
	}
	return &m, nil
}

func (s *server) getSite(ctx context.Context, in *idPath) (*Out[Site], error) {
	m, err := s.loadSite(ctx, s.db.WithContext(ctx), in.ID)
	if err != nil {
		return nil, err
	}
	out := []Site{toSite(m)}
	s.fillSiteClients(ctx, out)
	return &Out[Site]{Body: out[0]}, nil
}

func (s *server) createSite(ctx context.Context, in *struct{ Body SiteCreate }) (*Out[Site], error) {
	b := in.Body
	m := model.Site{AccountID: auth.AccountFrom(ctx).ID, ClientID: b.ClientID, Name: strings.TrimSpace(b.Name),
		Street: b.Street, City: b.City, Zip: b.Zip, Lat: b.Lat, Lng: b.Lng, AreaM2: b.AreaM2, LawnM2: b.LawnM2,
		HedgeBM: b.HedgeBM, AccessNote: b.AccessNote, Note: b.Note}
	if m.Name == "" {
		return nil, invalid("name", "name must not be empty")
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		c, err := s.loadClient(ctx, tx, b.ClientID)
		if err != nil {
			return invalid("client_id", "client not found")
		}
		if err := tx.Create(&m).Error; err != nil {
			return dbErr(err, "site")
		}
		return recordSimple(ctx, tx, events.SiteCreated, events.SubjectSite, m.ID,
			fmt.Sprintf("Zahrada %s klienta %s vytvořena", m.Name, c.Name), map[string]any{"client_id": c.ID})
	})
	if err != nil {
		return nil, err
	}
	out := []Site{toSite(&m)}
	s.fillSiteClients(ctx, out)
	return &Out[Site]{Body: out[0]}, nil
}

func (s *server) patchSite(ctx context.Context, in *struct {
	ID   uint `path:"id"`
	Body SitePatch
}) (*Out[Site], error) {
	var m *model.Site
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) (err error) {
		if m, err = s.loadSite(ctx, tx, in.ID); err != nil {
			return err
		}
		p := in.Body
		apply(&m.Name, trimmed(p.Name))
		apply(&m.Street, p.Street)
		apply(&m.City, p.City)
		apply(&m.Zip, p.Zip)
		if p.Lat != nil {
			m.Lat = p.Lat
		}
		if p.Lng != nil {
			m.Lng = p.Lng
		}
		if p.AreaM2 != nil {
			m.AreaM2 = p.AreaM2
		}
		if p.LawnM2 != nil {
			m.LawnM2 = p.LawnM2
		}
		if p.HedgeBM != nil {
			m.HedgeBM = p.HedgeBM
		}
		apply(&m.AccessNote, p.AccessNote)
		apply(&m.Note, p.Note)
		apply(&m.Archived, p.Archived)
		if m.Name == "" {
			return invalid("name", "name must not be empty")
		}
		if err := tx.Save(m).Error; err != nil {
			return dbErr(err, "site")
		}
		return recordSimple(ctx, tx, events.SiteUpdated, events.SubjectSite, m.ID, "Zahrada "+m.Name+" upravena", nil)
	})
	if err != nil {
		return nil, err
	}
	out := []Site{toSite(m)}
	s.fillSiteClients(ctx, out)
	return &Out[Site]{Body: out[0]}, nil
}

func (s *server) deleteSite(ctx context.Context, in *idPath) (*NoContent, error) {
	var files []string
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		m, err := s.loadSite(ctx, tx, in.ID)
		if err != nil {
			return err
		}
		var n int64
		if err := tx.Model(&model.Job{}).Scopes(inAccount(ctx)).Where("site_id = ?", m.ID).Count(&n).Error; err != nil {
			return dbErr(err, "job")
		}
		if n > 0 {
			return conflict(CodeHasJobs, "the garden has jobs; archive it instead")
		}
		if files, err = deleteOwnerAttachments(ctx, tx, model.OwnerSite, m.ID); err != nil {
			return err
		}
		if err := tx.Delete(m).Error; err != nil {
			return dbErr(err, "site")
		}
		return recordSimple(ctx, tx, events.SiteDeleted, events.SubjectSite, m.ID, "Zahrada "+m.Name+" smazána", nil)
	})
	if err != nil {
		return nil, err
	}
	s.removeFiles(ctx, files)
	return &NoContent{}, nil
}

// ---- contractors ----

func (s *server) listContractors(ctx context.Context, in *struct {
	PageParams
	Query  string `query:"query" maxLength:"100"`
	Active string `query:"active" enum:"true,false," doc:"Empty = all"`
}) (*Out[ListResponse[Contractor]], error) {
	q := searchFilter(s.scoped(ctx).Model(&model.Contractor{}), in.Query).Order("active DESC, LOWER(name), id")
	if in.Active != "" {
		q = q.Where("active = ?", in.Active == "true")
	}
	out, err := paginate(q, in.PageParams, toContractor)
	if err != nil {
		return nil, err
	}
	s.fillContractorUsers(ctx, out.Body.Items)
	return out, nil
}

func (s *server) fillContractorUsers(ctx context.Context, items []Contractor) {
	for i := range items {
		items[i].Login = "none"
	}
	ids := []uint{}
	for _, c := range items {
		if c.UserID != nil {
			ids = append(ids, *c.UserID)
		}
	}
	if len(ids) == 0 {
		return
	}
	var users []model.User
	if err := s.db.WithContext(ctx).Select("id", "email", "link_only").Where("id IN ?", ids).Find(&users).Error; err != nil {
		return
	}
	byID := map[uint]model.User{}
	for _, u := range users {
		byID[u.ID] = u
	}
	for i := range items {
		if items[i].UserID == nil {
			continue
		}
		if u := byID[*items[i].UserID]; u.LinkOnly {
			items[i].Login = "link"
		} else {
			items[i].Login, items[i].UserEmail = "email", u.Email
		}
	}
}

func (s *server) loadContractor(ctx context.Context, db *gorm.DB, id uint) (*model.Contractor, error) {
	var m model.Contractor
	if err := db.Scopes(inAccount(ctx)).First(&m, id).Error; err != nil {
		return nil, dbErr(err, "contractor")
	}
	return &m, nil
}

func (s *server) contractorOut(ctx context.Context, m *model.Contractor) Contractor {
	out := []Contractor{toContractor(m)}
	s.fillContractorUsers(ctx, out)
	return out[0]
}

func (s *server) getContractor(ctx context.Context, in *idPath) (*Out[Contractor], error) {
	m, err := s.loadContractor(ctx, s.db.WithContext(ctx), in.ID)
	if err != nil {
		return nil, err
	}
	return &Out[Contractor]{Body: s.contractorOut(ctx, m)}, nil
}

func (s *server) createContractor(ctx context.Context, in *struct{ Body ContractorCreate }) (*Out[Contractor], error) {
	b, acc := in.Body, auth.AccountFrom(ctx)
	m := model.Contractor{AccountID: acc.ID, Name: strings.TrimSpace(b.Name), Kind: defaultStr(b.Kind, "person"),
		RegistrationNo: strings.TrimSpace(b.RegistrationNo), Phone: strings.TrimSpace(b.Phone), Email: strings.TrimSpace(b.Email),
		HourlyRate: acc.DefaultHourlyRate, KmRate: acc.DefaultKmRate, Color: b.Color, Active: true, Note: b.Note}
	apply(&m.HourlyRate, b.HourlyRate)
	apply(&m.KmRate, b.KmRate)
	if m.Name == "" {
		return nil, invalid("name", "name must not be empty")
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&m).Error; err != nil {
			return dbErr(err, "contractor")
		}
		return recordSimple(ctx, tx, events.ContractorCreated, events.SubjectContractor, m.ID, "Dodavatel "+m.Name+" vytvořen", nil)
	})
	if err != nil {
		return nil, err
	}
	return &Out[Contractor]{Body: s.contractorOut(ctx, &m)}, nil
}

func (s *server) patchContractor(ctx context.Context, in *struct {
	ID   uint `path:"id"`
	Body ContractorPatch
}) (*Out[Contractor], error) {
	var m *model.Contractor
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) (err error) {
		if m, err = s.loadContractor(ctx, tx, in.ID); err != nil {
			return err
		}
		p := in.Body
		apply(&m.Name, trimmed(p.Name))
		apply(&m.Kind, p.Kind)
		apply(&m.RegistrationNo, trimmed(p.RegistrationNo))
		apply(&m.Phone, trimmed(p.Phone))
		apply(&m.Email, trimmed(p.Email))
		apply(&m.HourlyRate, p.HourlyRate)
		apply(&m.KmRate, p.KmRate)
		apply(&m.Color, p.Color)
		apply(&m.Active, p.Active)
		apply(&m.Note, p.Note)
		if m.Name == "" {
			return invalid("name", "name must not be empty")
		}
		if err := tx.Save(m).Error; err != nil {
			return dbErr(err, "contractor")
		}
		return recordSimple(ctx, tx, events.ContractorUpdated, events.SubjectContractor, m.ID, "Dodavatel "+m.Name+" upraven", nil)
	})
	if err != nil {
		return nil, err
	}
	return &Out[Contractor]{Body: s.contractorOut(ctx, m)}, nil
}

func (s *server) deleteContractor(ctx context.Context, in *idPath) (*NoContent, error) {
	var files []string
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		m, err := s.loadContractor(ctx, tx, in.ID)
		if err != nil {
			return err
		}
		var n int64
		if err := tx.Model(&model.AttendanceEntry{}).Scopes(inAccount(ctx)).Where("contractor_id = ?", m.ID).Count(&n).Error; err != nil {
			return dbErr(err, "attendance")
		}
		var np int64
		if err := tx.Model(&model.ContractorPayment{}).Scopes(inAccount(ctx)).Where("contractor_id = ?", m.ID).Count(&np).Error; err != nil {
			return dbErr(err, "payment")
		}
		if n > 0 || np > 0 {
			return conflict(CodeHasAttendance, "the contractor has attendance or payments; deactivate instead")
		}
		if err := tx.Where("contractor_id = ?", m.ID).Delete(&model.JobContractor{}).Error; err != nil {
			return dbErr(err, "job contractor")
		}
		if m.UserID != nil { // the login loses its contractor membership of this account
			if err := tx.Scopes(inAccount(ctx)).Where("user_id = ? AND role = ?", *m.UserID, model.RoleContractor).
				Delete(&model.Membership{}).Error; err != nil {
				return dbErr(err, "membership")
			}
			var u model.User
			if err := tx.First(&u, *m.UserID).Error; err == nil && u.LinkOnly { // link-only users exist only for this contractor
				for _, t := range []any{&model.Session{}, &model.LoginLink{}, &model.APIToken{}} {
					if err := tx.Where("user_id = ?", u.ID).Delete(t).Error; err != nil {
						return dbErr(err, "user")
					}
				}
				if err := tx.Delete(&u).Error; err != nil {
					return dbErr(err, "user")
				}
			}
		}
		if err := tx.Scopes(inAccount(ctx)).Where("contractor_id = ? AND accepted_at IS NULL", m.ID).Delete(&model.Invitation{}).Error; err != nil {
			return dbErr(err, "invitation")
		}
		if files, err = deleteOwnerAttachments(ctx, tx, model.OwnerContractor, m.ID); err != nil {
			return err
		}
		if err := tx.Delete(m).Error; err != nil {
			return dbErr(err, "contractor")
		}
		return recordSimple(ctx, tx, events.ContractorDeleted, events.SubjectContractor, m.ID, "Dodavatel "+m.Name+" smazán", nil)
	})
	if err != nil {
		return nil, err
	}
	s.removeFiles(ctx, files)
	return &NoContent{}, nil
}

// inviteContractor e-mails an invitation with role contractor; accepting it
// links the user to this contractor (members.go joinByInvitation).
func (s *server) inviteContractor(ctx context.Context, in *struct {
	ID   uint `path:"id"`
	Body ContractorInvite
}) (*Out[Invitation], error) {
	m, err := s.loadContractor(ctx, s.db.WithContext(ctx), in.ID)
	if err != nil {
		return nil, err
	}
	if m.UserID != nil {
		return nil, conflict(CodeAlreadyLinked, "the contractor already has a login")
	}
	email := strings.TrimSpace(defaultStr(in.Body.Email, m.Email))
	if email == "" || !strings.Contains(email, "@") {
		return nil, invalid("email", "an e-mail address is required")
	}
	out, err := s.createInvitation(ctx, email, model.RoleContractor, &m.ID)
	if err != nil {
		return nil, err
	}
	_ = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return recordSimple(ctx, tx, events.ContractorInvited, events.SubjectContractor, m.ID,
			"Dodavatel "+m.Name+" pozván do aplikace", map[string]any{"email": out.Body.Email})
	})
	return out, nil
}
