package api

// Price list (ceník) and quotes (kalkulace → nabídka) — SPEC §4.5–4.6.
// Totals are always computed by calc.Compute; job photos for contractors live at the end.

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
	"github.com/qwerin/zahrada/internal/calc"
	"github.com/qwerin/zahrada/internal/events"
	"github.com/qwerin/zahrada/internal/model"
)

// ---- price list ----

type PriceItem struct {
	ID         uint      `json:"id"`
	Kind       string    `json:"kind" enum:"work,material,machine,transport,subcontract,other"`
	Code       string    `json:"code"`
	Name       string    `json:"name"`
	Unit       string    `json:"unit"`
	Category   string    `json:"category"`
	UnitCost   int64     `json:"unit_cost"`
	UnitPrice  *int64    `json:"unit_price,omitempty"`
	MarkupBps  int32     `json:"markup_bps"`
	WasteBps   int32     `json:"waste_bps"`
	NormHours  string    `json:"norm_hours"`
	VatRateBps int32     `json:"vat_rate_bps"`
	SalePrice  int64     `json:"sale_price" doc:"Effective sale price per unit (unit_price or cost + markup)"`
	Archived   bool      `json:"archived"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type PriceItemInput struct {
	Kind       string `json:"kind" enum:"work,material,machine,transport,subcontract,other"`
	Code       string `json:"code,omitempty" maxLength:"50"`
	Name       string `json:"name" minLength:"1" maxLength:"300"`
	Unit       string `json:"unit,omitempty" maxLength:"20"`
	Category   string `json:"category,omitempty" maxLength:"100"`
	UnitCost   int64  `json:"unit_cost,omitempty" minimum:"0"`
	UnitPrice  *int64 `json:"unit_price,omitempty" minimum:"0" doc:"Omitted = unit_cost + markup"`
	MarkupBps  int32  `json:"markup_bps,omitempty" minimum:"0" maximum:"100000"`
	WasteBps   int32  `json:"waste_bps,omitempty" minimum:"0" maximum:"10000"`
	NormHours  string `json:"norm_hours,omitempty" doc:"Labour hours per unit, decimal string"`
	VatRateBps *int32 `json:"vat_rate_bps,omitempty" minimum:"0" maximum:"10000" doc:"Omitted = account default"`
	Archived   *bool  `json:"archived,omitempty"`
}

type PriceItemImport struct {
	Items []PriceItemInput `json:"items" minItems:"1" maxItems:"500"`
}

func toPriceItem(m *model.PriceItem) PriceItem {
	sale := calc.ApplyBps(m.UnitCost, m.MarkupBps)
	if m.UnitPrice != nil {
		sale = *m.UnitPrice
	}
	return PriceItem{ID: m.ID, Kind: m.Kind, Code: m.Code, Name: m.Name, Unit: m.Unit, Category: m.Category,
		UnitCost: m.UnitCost, UnitPrice: m.UnitPrice, MarkupBps: m.MarkupBps, WasteBps: m.WasteBps,
		NormHours: calc.FormatMilli(m.NormHoursMilli), VatRateBps: m.VatRateBps, SalePrice: sale,
		Archived: m.Archived, UpdatedAt: m.UpdatedAt}
}

func (s *server) applyPriceItem(ctx context.Context, m *model.PriceItem, in PriceItemInput, field string) error {
	if !calc.Kind(in.Kind).Valid() {
		return invalid(field+"kind", "unknown kind")
	}
	norm := int64(0)
	if strings.TrimSpace(in.NormHours) != "" {
		v, err := calc.ParseMilli(in.NormHours)
		if err != nil || v < 0 {
			return invalid(field+"norm_hours", "norm_hours must be a number with at most 3 decimals")
		}
		norm = v
	}
	m.Kind, m.Code, m.Name, m.Unit, m.Category = in.Kind, strings.TrimSpace(in.Code), strings.TrimSpace(in.Name), strings.TrimSpace(in.Unit), strings.TrimSpace(in.Category)
	m.UnitCost, m.UnitPrice, m.MarkupBps, m.WasteBps, m.NormHoursMilli = in.UnitCost, in.UnitPrice, in.MarkupBps, in.WasteBps, norm
	m.VatRateBps = auth.AccountFrom(ctx).DefaultVatRateBps
	apply(&m.VatRateBps, in.VatRateBps)
	apply(&m.Archived, in.Archived)
	if m.Name == "" {
		return invalid(field+"name", "name must not be empty")
	}
	return nil
}

func (s *server) registerQuotes(g huma.API) {
	huma.Get(g, "/price-items", s.listPriceItems)
	huma.Post(g, "/price-items", s.createPriceItem, status(http.StatusCreated), auth.ForEditors)
	huma.Post(g, "/price-items/import", s.importPriceItems, status(http.StatusCreated), auth.ForEditors)
	huma.Put(g, "/price-items/{id}", s.updatePriceItem, auth.ForEditors)
	huma.Delete(g, "/price-items/{id}", s.deletePriceItem, status(http.StatusNoContent), auth.ForEditors)

	huma.Get(g, "/jobs/{id}/quotes", s.listQuotes)
	huma.Post(g, "/jobs/{id}/quotes", s.createQuote, status(http.StatusCreated), auth.ForEditors)
	huma.Get(g, "/quotes/{id}", s.getQuote)
	huma.Put(g, "/quotes/{id}", s.updateQuote, auth.ForEditors)
	huma.Post(g, "/quotes/{id}/status", s.changeQuoteStatus, auth.ForEditors)
	huma.Delete(g, "/quotes/{id}", s.deleteQuote, status(http.StatusNoContent), auth.ForEditors)

	huma.Get(g, "/jobs/{id}/photos", s.staffJobPhotos)
	huma.Get(g, "/me/jobs/{id}/photos", s.meJobPhotos, auth.ForEveryone)
	huma.Post(g, "/me/jobs/{id}/photos", s.meUploadPhoto, status(http.StatusCreated), auth.ForEveryone, slowUpload(5*time.Minute))
	huma.Get(g, "/me/photos/{id}", s.mePhoto, auth.ForEveryone, func(o *huma.Operation) {
		o.Responses = map[string]*huma.Response{"200": {Description: "Photo content"}}
	})
}

func (s *server) listPriceItems(ctx context.Context, in *struct {
	PageParams
	Query    string `query:"query" maxLength:"100"`
	Kind     string `query:"kind" enum:"work,material,machine,transport,subcontract,other,"`
	Archived bool   `query:"archived"`
}) (*Out[ListResponse[PriceItem]], error) {
	q := searchFilter(s.scoped(ctx).Model(&model.PriceItem{}), in.Query).Where("archived = ?", in.Archived).Order("LOWER(category), LOWER(name), id")
	if in.Kind != "" {
		q = q.Where("kind = ?", in.Kind)
	}
	return paginate(q, in.PageParams, toPriceItem)
}

func (s *server) createPriceItem(ctx context.Context, in *struct{ Body PriceItemInput }) (*Out[PriceItem], error) {
	m := model.PriceItem{AccountID: auth.AccountFrom(ctx).ID}
	if err := s.applyPriceItem(ctx, &m, in.Body, ""); err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Create(&m).Error; err != nil {
		return nil, dbErr(err, "price item")
	}
	return &Out[PriceItem]{Body: toPriceItem(&m)}, nil
}

// importPriceItems creates many items at once (paste from a spreadsheet or a chat).
func (s *server) importPriceItems(ctx context.Context, in *struct{ Body PriceItemImport }) (*Out[ListResponse[PriceItem]], error) {
	items := make([]model.PriceItem, len(in.Body.Items))
	for i, it := range in.Body.Items {
		items[i].AccountID = auth.AccountFrom(ctx).ID
		if err := s.applyPriceItem(ctx, &items[i], it, fmt.Sprintf("items[%d].", i)); err != nil {
			return nil, err
		}
	}
	if err := s.db.WithContext(ctx).Create(&items).Error; err != nil {
		return nil, dbErr(err, "price item")
	}
	out := make([]PriceItem, len(items))
	for i := range items {
		out[i] = toPriceItem(&items[i])
	}
	return &Out[ListResponse[PriceItem]]{Body: ListResponse[PriceItem]{Items: out, Page: 1, PerPage: len(out), Total: int64(len(out))}}, nil
}

func (s *server) updatePriceItem(ctx context.Context, in *struct {
	ID   uint `path:"id"`
	Body PriceItemInput
}) (*Out[PriceItem], error) {
	var m model.PriceItem
	if err := s.scoped(ctx).First(&m, in.ID).Error; err != nil {
		return nil, dbErr(err, "price item")
	}
	if err := s.applyPriceItem(ctx, &m, in.Body, ""); err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Save(&m).Error; err != nil {
		return nil, dbErr(err, "price item")
	}
	return &Out[PriceItem]{Body: toPriceItem(&m)}, nil
}

func (s *server) deletePriceItem(ctx context.Context, in *idPath) (*NoContent, error) {
	res := s.scoped(ctx).Delete(&model.PriceItem{}, in.ID)
	if res.Error != nil {
		return nil, dbErr(res.Error, "price item")
	}
	if res.RowsAffected == 0 {
		return nil, notFound("price item")
	}
	// quotes keep their copied values; only the link goes away
	_ = s.scoped(ctx).Model(&model.QuoteItem{}).Where("price_item_id = ?", in.ID).Update("price_item_id", nil).Error
	return &NoContent{}, nil
}

// ---- quotes ----

type QuoteLine struct {
	CalcItem
	Name        string `json:"name" minLength:"1" maxLength:"300"`
	Unit        string `json:"unit,omitempty" maxLength:"20"`
	PriceItemID *uint  `json:"price_item_id,omitempty"`
	Note        string `json:"note,omitempty" maxLength:"1000"`
}

type QuoteInput struct {
	Title       string      `json:"title" minLength:"1" maxLength:"300"`
	ValidUntil  string      `json:"valid_until,omitempty" doc:"YYYY-MM-DD"`
	DiscountBps int32       `json:"discount_bps,omitempty" minimum:"0" maximum:"10000"`
	Intro       string      `json:"intro,omitempty" maxLength:"5000"`
	Note        string      `json:"note,omitempty" maxLength:"5000"`
	Items       []QuoteLine `json:"items" maxItems:"500" nullable:"false"`
}

type QuoteSummary struct {
	ID         uint       `json:"id"`
	JobID      uint       `json:"job_id"`
	Number     string     `json:"number" example:"Z2026-001/v2"`
	Version    int        `json:"version"`
	Title      string     `json:"title"`
	Status     string     `json:"status" enum:"draft,sent,accepted,rejected,superseded"`
	ValidUntil string     `json:"valid_until,omitempty"`
	Net        int64      `json:"net"`
	Gross      int64      `json:"gross"`
	Cost       int64      `json:"cost"`
	Margin     int64      `json:"margin"`
	MarginBps  int32      `json:"margin_bps"`
	Hours      string     `json:"hours"`
	SentAt     *time.Time `json:"sent_at,omitempty"`
	AcceptedAt *time.Time `json:"accepted_at,omitempty"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

type Quote struct {
	QuoteSummary
	DiscountBps int32       `json:"discount_bps"`
	Intro       string      `json:"intro"`
	Note        string      `json:"note"`
	Items       []QuoteLine `json:"items" nullable:"false"`
	Result      CalcResult  `json:"result" doc:"Computed lines and totals"`
	Job         QuoteJob    `json:"job"`
}

// QuoteJob is what the printed offer needs to know about the job and the client.
type QuoteJob struct {
	ID            uint   `json:"id"`
	Number        string `json:"number"`
	Title         string `json:"title"`
	Status        string `json:"status"`
	ClientName    string `json:"client_name"`
	ClientAddress string `json:"client_address"`
	ClientEmail   string `json:"client_email"`
	ClientPhone   string `json:"client_phone"`
	ClientRegNo   string `json:"client_registration_no"`
	SiteName      string `json:"site_name,omitempty"`
	SiteAddress   string `json:"site_address,omitempty"`
}

type QuoteCreate struct {
	CopyFrom *uint  `json:"copy_from,omitempty" doc:"Quote version to copy (new version of the offer)"`
	Title    string `json:"title,omitempty" maxLength:"300"`
}

type QuoteStatusChange struct {
	Status string `json:"status" enum:"draft,sent,accepted,rejected"`
}

var quoteTransitions = map[string][]string{
	model.QuoteDraft:    {model.QuoteSent, model.QuoteAccepted},
	model.QuoteSent:     {model.QuoteDraft, model.QuoteAccepted, model.QuoteRejected},
	model.QuoteRejected: {model.QuoteDraft},
}

func quoteNumber(job *model.Job, version int) string {
	return fmt.Sprintf("%s/v%d", job.Number, version)
}

func toQuoteSummary(m *model.Quote, job *model.Job) QuoteSummary {
	return QuoteSummary{ID: m.ID, JobID: m.JobID, Number: quoteNumber(job, m.Version), Version: m.Version, Title: m.Title,
		Status: m.Status, ValidUntil: m.ValidUntil, Net: m.Net, Gross: m.Gross, Cost: m.Cost, Margin: m.Margin,
		MarginBps: m.MarginBps, Hours: calc.FormatMilli(m.HoursMilli), SentAt: m.SentAt, AcceptedAt: m.AcceptedAt, UpdatedAt: m.UpdatedAt}
}

func toLines(items []model.QuoteItem) []QuoteLine {
	out := make([]QuoteLine, len(items))
	for i, it := range items {
		out[i] = QuoteLine{CalcItem: CalcItem{Kind: it.Kind, Section: it.Section, Quantity: calc.FormatMilli(it.QuantityMilli),
			WasteBps: it.WasteBps, UnitCost: it.UnitCost, UnitPrice: it.UnitPrice, MarkupBps: it.MarkupBps,
			DiscountBps: it.DiscountBps, VatRateBps: it.VatRateBps, NormHours: calc.FormatMilli(it.NormHoursMilli)},
			Name: it.Name, Unit: it.Unit, PriceItemID: it.PriceItemID, Note: it.Note}
	}
	return out
}

// computeQuote recalculates the stored totals of q from its items.
func (s *server) computeQuote(ctx context.Context, q *model.Quote) (calc.Totals, error) {
	ci := make([]calc.Item, len(q.Items))
	for i, it := range q.Items {
		ci[i] = calc.Item{Kind: calc.Kind(it.Kind), Section: it.Section, QuantityMilli: it.QuantityMilli, WasteBps: it.WasteBps,
			UnitCost: it.UnitCost, UnitPrice: it.UnitPrice, MarkupBps: it.MarkupBps, DiscountBps: it.DiscountBps,
			VATRateBps: it.VatRateBps, NormHoursMilli: it.NormHoursMilli}
	}
	t, err := calc.Compute(ci, calc.Options{VATPayer: auth.AccountFrom(ctx).VatPayer, DiscountBps: q.DiscountBps})
	if err != nil {
		return t, computeErr(err)
	}
	q.Net, q.VATTotal, q.Gross, q.Cost, q.Margin, q.MarginBps, q.HoursMilli = t.Net, t.VATTotal, t.Gross, t.Cost, t.Margin, t.MarginBps, t.HoursMilli
	return t, nil
}

func (s *server) loadQuote(ctx context.Context, db *gorm.DB, id uint) (*model.Quote, *model.Job, error) {
	var q model.Quote
	if err := db.Scopes(inAccount(ctx)).Preload("Items", func(d *gorm.DB) *gorm.DB { return d.Order("position") }).First(&q, id).Error; err != nil {
		return nil, nil, dbErr(err, "quote")
	}
	job, err := s.loadJob(ctx, db, q.JobID)
	return &q, job, err
}

func (s *server) quoteOut(ctx context.Context, db *gorm.DB, q *model.Quote, job *model.Job) (Quote, error) {
	t, err := s.computeQuote(ctx, q)
	if err != nil {
		return Quote{}, err
	}
	out := Quote{QuoteSummary: toQuoteSummary(q, job), DiscountBps: q.DiscountBps, Intro: q.Intro, Note: q.Note,
		Items: toLines(q.Items), Result: toCalcResult(t, auth.AccountFrom(ctx).VatPayer),
		Job: QuoteJob{ID: job.ID, Number: job.Number, Title: job.Title, Status: job.Status}}
	if c, err := s.loadClient(ctx, db, job.ClientID); err == nil {
		out.Job.ClientName, out.Job.ClientEmail, out.Job.ClientPhone, out.Job.ClientRegNo = c.Name, c.Email, c.Phone, c.RegistrationNo
		out.Job.ClientAddress = joinNonEmpty(c.Street, strings.TrimSpace(c.Zip+" "+c.City))
	}
	if job.SiteID != nil {
		if st, err := s.loadSite(ctx, db, *job.SiteID); err == nil {
			out.Job.SiteName, out.Job.SiteAddress = st.Name, joinNonEmpty(st.Street, strings.TrimSpace(st.Zip+" "+st.City))
		}
	}
	return out, nil
}

func (s *server) listQuotes(ctx context.Context, in *idPath) (*Out[[]QuoteSummary], error) {
	db := s.db.WithContext(ctx)
	job, err := s.loadJob(ctx, db, in.ID)
	if err != nil {
		return nil, err
	}
	var qs []model.Quote
	if err := db.Scopes(inAccount(ctx)).Where("job_id = ?", job.ID).Order("version DESC").Find(&qs).Error; err != nil {
		return nil, dbErr(err, "quote")
	}
	out := make([]QuoteSummary, len(qs))
	for i := range qs {
		out[i] = toQuoteSummary(&qs[i], job)
	}
	return &Out[[]QuoteSummary]{Body: out}, nil
}

func (s *server) createQuote(ctx context.Context, in *struct {
	ID   uint `path:"id"`
	Body QuoteCreate
}) (*Out[Quote], error) {
	var out Quote
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		job, err := s.loadJob(ctx, tx, in.ID)
		if err != nil {
			return err
		}
		acc := auth.AccountFrom(ctx)
		var last int
		if err := tx.Model(&model.Quote{}).Scopes(inAccount(ctx)).Where("job_id = ?", job.ID).Select("COALESCE(MAX(version), 0)").Scan(&last).Error; err != nil {
			return dbErr(err, "quote")
		}
		q := model.Quote{AccountID: acc.ID, JobID: job.ID, Version: last + 1, Title: defaultStr(strings.TrimSpace(in.Body.Title), job.Title),
			Status: model.QuoteDraft, Intro: "", Note: acc.QuoteTerms}
		if acc.QuoteValidityDays > 0 {
			q.ValidUntil = s.deps.Now().In(s.accountLocation(ctx)).AddDate(0, 0, acc.QuoteValidityDays).Format(time.DateOnly)
		}
		if in.Body.CopyFrom != nil {
			src, _, err := s.loadQuote(ctx, tx, *in.Body.CopyFrom)
			if err != nil || src.JobID != job.ID {
				return invalid("copy_from", "quote not found in this job")
			}
			q.Title, q.DiscountBps, q.Intro, q.Note = src.Title, src.DiscountBps, src.Intro, src.Note
			for _, it := range src.Items {
				it.ID, it.QuoteID = 0, 0
				q.Items = append(q.Items, it)
			}
			// older open versions are replaced by the new one
			if err := tx.Model(&model.Quote{}).Scopes(inAccount(ctx)).Where("job_id = ? AND status IN ?", job.ID,
				[]string{model.QuoteDraft, model.QuoteSent}).Update("status", model.QuoteSuperseded).Error; err != nil {
				return dbErr(err, "quote")
			}
		}
		if _, err := s.computeQuote(ctx, &q); err != nil {
			return err
		}
		if err := tx.Create(&q).Error; err != nil {
			return dbErr(err, "quote")
		}
		if err := recordSimple(ctx, tx, events.JobUpdated, events.SubjectJob, job.ID,
			fmt.Sprintf("Kalkulace %s vytvořena", quoteNumber(job, q.Version)), map[string]any{"quote_id": q.ID}); err != nil {
			return err
		}
		out, err = s.quoteOut(ctx, tx, &q, job)
		return err
	})
	if err != nil {
		return nil, err
	}
	return &Out[Quote]{Body: out}, nil
}

func (s *server) getQuote(ctx context.Context, in *idPath) (*Out[Quote], error) {
	db := s.db.WithContext(ctx)
	q, job, err := s.loadQuote(ctx, db, in.ID)
	if err != nil {
		return nil, err
	}
	out, err := s.quoteOut(ctx, db, q, job)
	return &Out[Quote]{Body: out}, err
}

func (s *server) updateQuote(ctx context.Context, in *struct {
	ID   uint `path:"id"`
	Body QuoteInput
}) (*Out[Quote], error) {
	b := in.Body
	if b.ValidUntil != "" && !validDate(b.ValidUntil) {
		return nil, invalid("valid_until", "use YYYY-MM-DD")
	}
	calcItems := make([]CalcItem, len(b.Items))
	for i, l := range b.Items {
		calcItems[i] = l.CalcItem
		if strings.TrimSpace(l.Name) == "" {
			return nil, invalid(fmt.Sprintf("items[%d].name", i), "name must not be empty")
		}
	}
	parsed, err := toCalcItems(calcItems)
	if err != nil {
		return nil, err
	}
	var out Quote
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		q, job, err := s.loadQuote(ctx, tx, in.ID)
		if err != nil {
			return err
		}
		if q.Status != model.QuoteDraft {
			return conflict(CodeNotEditable, "only a draft can be edited; return it to draft or create a new version")
		}
		q.Title, q.ValidUntil, q.DiscountBps, q.Intro, q.Note = strings.TrimSpace(b.Title), b.ValidUntil, b.DiscountBps, b.Intro, b.Note
		q.Items = make([]model.QuoteItem, len(b.Items))
		for i, l := range b.Items {
			p := parsed[i]
			q.Items[i] = model.QuoteItem{QuoteID: q.ID, AccountID: q.AccountID, Position: i + 1, Section: strings.TrimSpace(l.Section),
				Kind: string(p.Kind), PriceItemID: l.PriceItemID, Name: strings.TrimSpace(l.Name), Unit: strings.TrimSpace(l.Unit),
				QuantityMilli: p.QuantityMilli, WasteBps: p.WasteBps, UnitCost: p.UnitCost, UnitPrice: p.UnitPrice,
				MarkupBps: p.MarkupBps, DiscountBps: p.DiscountBps, VatRateBps: p.VATRateBps, NormHoursMilli: p.NormHoursMilli, Note: l.Note}
		}
		if _, err := s.computeQuote(ctx, q); err != nil {
			return err
		}
		if err := tx.Where("quote_id = ?", q.ID).Delete(&model.QuoteItem{}).Error; err != nil {
			return dbErr(err, "quote item")
		}
		if len(q.Items) > 0 {
			if err := tx.Create(&q.Items).Error; err != nil {
				return dbErr(err, "quote item")
			}
		}
		if err := tx.Omit("Items").Save(q).Error; err != nil {
			return dbErr(err, "quote")
		}
		out, err = s.quoteOut(ctx, tx, q, job)
		return err
	})
	if err != nil {
		return nil, err
	}
	return &Out[Quote]{Body: out}, nil
}

func (s *server) changeQuoteStatus(ctx context.Context, in *struct {
	ID   uint `path:"id"`
	Body QuoteStatusChange
}) (*Out[Quote], error) {
	to := in.Body.Status
	var out Quote
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		q, job, err := s.loadQuote(ctx, tx, in.ID)
		if err != nil {
			return err
		}
		if !slices.Contains(quoteTransitions[q.Status], to) {
			return conflict(CodeInvalidTransition, fmt.Sprintf("a quote cannot go from %s to %s", q.Status, to))
		}
		now := s.deps.Now()
		upd := map[string]any{"status": to}
		switch to {
		case model.QuoteSent:
			upd["sent_at"] = now
			if job.Status == model.JobInquiry {
				if err := setJobStatus(ctx, tx, job, model.JobQuoted); err != nil {
					return err
				}
			}
		case model.QuoteAccepted:
			upd["accepted_at"] = now
			if err := tx.Model(&model.Quote{}).Scopes(inAccount(ctx)).Where("job_id = ? AND id <> ? AND status IN ?", job.ID, q.ID,
				[]string{model.QuoteDraft, model.QuoteSent}).Update("status", model.QuoteSuperseded).Error; err != nil {
				return dbErr(err, "quote")
			}
			if err := tx.Model(job).Update("accepted_quote_id", q.ID).Error; err != nil {
				return dbErr(err, "job")
			}
			if job.Status == model.JobInquiry || job.Status == model.JobQuoted {
				if err := setJobStatus(ctx, tx, job, model.JobOrdered); err != nil {
					return err
				}
			}
		}
		if err := tx.Model(q).Updates(upd).Error; err != nil {
			return dbErr(err, "quote")
		}
		q.Status = to
		labels := map[string]string{model.QuoteDraft: "vrácena k úpravám", model.QuoteSent: "odeslána klientovi",
			model.QuoteAccepted: "přijata", model.QuoteRejected: "odmítnuta"}
		if err := recordSimple(ctx, tx, events.JobUpdated, events.SubjectJob, job.ID,
			fmt.Sprintf("Nabídka %s %s", quoteNumber(job, q.Version), labels[to]), map[string]any{"quote_id": q.ID, "status": to}); err != nil {
			return err
		}
		out, err = s.quoteOut(ctx, tx, q, job)
		return err
	})
	if err != nil {
		return nil, err
	}
	return &Out[Quote]{Body: out}, nil
}

func (s *server) deleteQuote(ctx context.Context, in *idPath) (*NoContent, error) {
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		q, _, err := s.loadQuote(ctx, tx, in.ID)
		if err != nil {
			return err
		}
		if q.Status == model.QuoteAccepted {
			return conflict(CodeNotEditable, "an accepted offer cannot be deleted")
		}
		if err := tx.Where("quote_id = ?", q.ID).Delete(&model.QuoteItem{}).Error; err != nil {
			return dbErr(err, "quote item")
		}
		return dbErrOrNil(tx.Delete(q).Error, "quote")
	})
	if err != nil {
		return nil, err
	}
	return &NoContent{}, nil
}

// ---- job photos for contractors ----

type PhotoUpload struct {
	File huma.FormFile `form:"file" required:"true" doc:"JPEG, PNG, WebP or HEIC, max 20 MB"`
}

// meJob returns the job if it is assigned to the current user's contractor.
func (s *server) meJob(ctx context.Context, jobID uint) (*model.Job, error) {
	db := s.db.WithContext(ctx)
	c, err := s.meContractorOf(ctx, db)
	if err != nil {
		return nil, err
	}
	var n int64
	if err := db.Model(&model.JobContractor{}).Where("job_id = ? AND contractor_id = ?", jobID, c.ID).Count(&n).Error; err != nil {
		return nil, dbErr(err, "job contractor")
	}
	if n == 0 {
		return nil, notFound("job")
	}
	return s.loadJob(ctx, db, jobID)
}

// jobPhotos lists the image attachments of a job with uploader names.
func (s *server) jobPhotos(ctx context.Context, jobID uint) ([]Attachment, error) {
	var rows []model.Attachment
	if err := s.scoped(ctx).Where("owner_type = ? AND owner_id = ? AND content_type LIKE ?", model.OwnerJob, jobID, "image/%").
		Order("id DESC").Find(&rows).Error; err != nil {
		return nil, dbErr(err, "attachment")
	}
	ids := []uint{}
	for _, a := range rows {
		if a.UploadedBy != nil {
			ids = append(ids, *a.UploadedBy)
		}
	}
	names := map[uint]string{}
	if len(ids) > 0 {
		var users []model.User
		_ = s.db.WithContext(ctx).Select("id", "name", "email").Where("id IN ?", ids).Find(&users).Error
		for _, u := range users {
			names[u.ID] = defaultStr(u.Name, u.Email)
		}
	}
	out := make([]Attachment, len(rows))
	for i := range rows {
		out[i] = toAttachment(&rows[i])
		if rows[i].UploadedBy != nil {
			out[i].UploadedBy = names[*rows[i].UploadedBy]
		}
	}
	return out, nil
}

func (s *server) staffJobPhotos(ctx context.Context, in *idPath) (*Out[[]Attachment], error) {
	if _, err := s.loadJob(ctx, s.db.WithContext(ctx), in.ID); err != nil {
		return nil, err
	}
	out, err := s.jobPhotos(ctx, in.ID)
	return &Out[[]Attachment]{Body: out}, err
}

func (s *server) meJobPhotos(ctx context.Context, in *idPath) (*Out[[]Attachment], error) {
	if _, err := s.meJob(ctx, in.ID); err != nil {
		return nil, err
	}
	out, err := s.jobPhotos(ctx, in.ID)
	return &Out[[]Attachment]{Body: out}, err
}

func (s *server) meUploadPhoto(ctx context.Context, in *struct {
	ID      uint `path:"id"`
	RawBody huma.MultipartFormFiles[PhotoUpload]
}) (*Out[Attachment], error) {
	if _, err := s.meJob(ctx, in.ID); err != nil {
		return nil, err
	}
	f := in.RawBody.Data().File
	if !f.IsSet || f.File == nil {
		return nil, invalid("file", "file is required")
	}
	defer f.Close()
	if f.Size > MaxAttachmentSize {
		return nil, huma.NewError(http.StatusRequestEntityTooLarge, "file is larger than 20 MB")
	}
	if f.Size == 0 {
		return nil, invalid("file", "file is empty")
	}
	m, err := s.storeUpload(ctx, f, model.OwnerJob, in.ID, true)
	if err != nil {
		return nil, err
	}
	return &Out[Attachment]{Body: toAttachment(m)}, nil
}

// mePhoto streams a photo of a job assigned to the current contractor.
func (s *server) mePhoto(ctx context.Context, in *idPath) (*huma.StreamResponse, error) {
	m, err := s.attachment(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	if m.OwnerType != model.OwnerJob || !strings.HasPrefix(m.ContentType, "image/") {
		return nil, notFound("photo")
	}
	if _, err := s.meJob(ctx, m.OwnerID); err != nil {
		var se huma.StatusError
		if errors.As(err, &se) && se.GetStatus() == http.StatusNotFound {
			return nil, notFound("photo")
		}
		return nil, err
	}
	return s.streamAttachment(ctx, m, true)
}
