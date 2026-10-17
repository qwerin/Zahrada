package api

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg" // image.DecodeConfig formats
	_ "image/png"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"gorm.io/gorm"

	"github.com/qwerin/zahrada/internal/auth"
	"github.com/qwerin/zahrada/internal/model"
)

// Account is the company profile and settings (SPEC §4.1).
type Account struct {
	Slug                    string       `json:"slug"`
	Role                    string       `json:"role" enum:"owner,admin,accountant,member,contractor" doc:"Current user's role"`
	Name                    string       `json:"name"`
	RegistrationNo          string       `json:"registration_no"`
	VatNo                   string       `json:"vat_no"`
	Street                  string       `json:"street"`
	City                    string       `json:"city"`
	Zip                     string       `json:"zip"`
	Country                 string       `json:"country"`
	Email                   string       `json:"email"`
	Phone                   string       `json:"phone"`
	Web                     string       `json:"web"`
	VatPayer                bool         `json:"vat_payer"`
	DefaultVatRateBps       int32        `json:"default_vat_rate_bps"`
	DefaultHourlyRate       int64        `json:"default_hourly_rate" doc:"Default contractor rate, haléře per hour"`
	DefaultKmRate           int64        `json:"default_km_rate" doc:"Default rate per km, haléře"`
	AttendanceRoundingMin   int          `json:"attendance_rounding_min" enum:"0,5,10,15,30"`
	AttendanceRoundingMode  string       `json:"attendance_rounding_mode" enum:"nearest,up,down"`
	AttendanceLockedThrough string       `json:"attendance_locked_through" doc:"Attendance is closed through this day (YYYY-MM-DD), empty = open"`
	AttendanceGPS           bool         `json:"attendance_gps"`
	PlanHorizonWeeks        int          `json:"plan_horizon_weeks"`
	QuoteValidityDays       int          `json:"quote_validity_days"`
	QuoteTerms              string       `json:"quote_terms"`
	TimeZone                string       `json:"time_zone" doc:"IANA zone of the account (attendance, plan)"`
	LogoAttachmentID        *uint        `json:"logo_attachment_id,omitempty" doc:"Logo image attachment (PNG/JPEG)"`
	OnboardedAt             *time.Time   `json:"onboarded_at,omitempty" doc:"When the onboarding was finished (null = show onboarding)"`
	Capabilities            Capabilities `json:"capabilities" doc:"What the current user's role may do (derived from role)"`
	CreatedAt               time.Time    `json:"created_at"`
	UpdatedAt               time.Time    `json:"updated_at"`
}

type AccountCreate struct {
	Name string `json:"name" minLength:"1" maxLength:"200"`
}

// AccountPatch: nil fields are left unchanged.
type AccountPatch struct {
	Name                   *string `json:"name,omitempty" minLength:"1" maxLength:"200"`
	RegistrationNo         *string `json:"registration_no,omitempty" maxLength:"20"`
	VatNo                  *string `json:"vat_no,omitempty" maxLength:"20"`
	Street                 *string `json:"street,omitempty" maxLength:"200"`
	City                   *string `json:"city,omitempty" maxLength:"100"`
	Zip                    *string `json:"zip,omitempty" maxLength:"20"`
	Country                *string `json:"country,omitempty" pattern:"^[A-Z]{2}$" doc:"ISO 3166-1 alpha-2"`
	Email                  *string `json:"email,omitempty" maxLength:"254"`
	Phone                  *string `json:"phone,omitempty" maxLength:"50"`
	Web                    *string `json:"web,omitempty" maxLength:"200"`
	VatPayer               *bool   `json:"vat_payer,omitempty"`
	DefaultVatRateBps      *int32  `json:"default_vat_rate_bps,omitempty" minimum:"0" maximum:"10000"`
	DefaultHourlyRate      *int64  `json:"default_hourly_rate,omitempty" minimum:"0"`
	DefaultKmRate          *int64  `json:"default_km_rate,omitempty" minimum:"0"`
	AttendanceRoundingMin  *int    `json:"attendance_rounding_min,omitempty" enum:"0,5,10,15,30"`
	AttendanceRoundingMode *string `json:"attendance_rounding_mode,omitempty" enum:"nearest,up,down"`
	AttendanceGPS          *bool   `json:"attendance_gps,omitempty"`
	PlanHorizonWeeks       *int    `json:"plan_horizon_weeks,omitempty" minimum:"1" maximum:"52"`
	QuoteValidityDays      *int    `json:"quote_validity_days,omitempty" minimum:"1" maximum:"365"`
	QuoteTerms             *string `json:"quote_terms,omitempty" maxLength:"5000"`
	TimeZone               *string `json:"time_zone,omitempty" maxLength:"64" doc:"IANA zone, empty = server default"`
	LogoAttachmentID       *uint   `json:"logo_attachment_id,omitempty" doc:"PNG/JPEG attachment of this account; 0 removes the logo"`
	Onboarded              *bool   `json:"onboarded,omitempty" doc:"true marks the onboarding as finished (sets onboarded_at), false shows it again"`
}

func toAccount(a *model.Account, role string) Account {
	return Account{
		Slug: a.Slug, Role: role, Name: a.Name, RegistrationNo: a.RegistrationNo, VatNo: a.VatNo,
		Street: a.Street, City: a.City, Zip: a.Zip, Country: a.Country, Email: a.Email, Phone: a.Phone, Web: a.Web,
		VatPayer: a.VatPayer, DefaultVatRateBps: a.DefaultVatRateBps, DefaultHourlyRate: a.DefaultHourlyRate,
		DefaultKmRate: a.DefaultKmRate, AttendanceRoundingMin: a.AttendanceRoundingMin,
		AttendanceRoundingMode:  defaultStr(a.AttendanceRoundingMode, model.RoundNearest),
		AttendanceLockedThrough: a.AttendanceLockedThrough, AttendanceGPS: a.AttendanceGPS,
		PlanHorizonWeeks: a.PlanHorizonWeeks, QuoteValidityDays: a.QuoteValidityDays, QuoteTerms: a.QuoteTerms,
		TimeZone: a.TimeZone, LogoAttachmentID: a.LogoAttachmentID,
		OnboardedAt: a.OnboardedAt, Capabilities: capabilitiesOf(role), CreatedAt: a.CreatedAt, UpdatedAt: a.UpdatedAt,
	}
}

// Capabilities are the permissions of a role (SPEC §3) for the client UI;
// the server enforces them per operation independently.
type Capabilities struct {
	Staff          bool `json:"staff" doc:"The firm's own people (false = contractor, who only sees own work)"`
	Edit           bool `json:"edit" doc:"Create/modify clients, sites, quotes, jobs, plan, attendance of others"`
	ManageSettings bool `json:"manage_settings" doc:"Account settings, closing attendance"`
	ManageMembers  bool `json:"manage_members" doc:"Invite and manage members"`
	ManageOwners   bool `json:"manage_owners" doc:"Add/remove owners and change their role"`
	ViewReports    bool `json:"view_reports" doc:"Reports and exports"`
}

func capabilitiesOf(role string) Capabilities {
	owner, admin := role == model.RoleOwner, role == model.RoleAdmin
	editor := owner || admin || role == model.RoleMember
	return Capabilities{Staff: role != model.RoleContractor && role != "", Edit: editor,
		ManageSettings: owner || admin, ManageMembers: owner || admin, ManageOwners: owner,
		ViewReports: editor || role == model.RoleAccountant}
}

// newAccount creates an account with default settings and makes ownerID its
// owner. Must run inside a transaction.
func newAccount(tx *gorm.DB, name string, ownerID uint) (*model.Account, error) {
	name = strings.TrimSpace(name)
	slug, err := uniqueSlug(tx, name)
	if err != nil {
		return nil, dbErr(err, "account")
	}
	acc := model.Account{
		Slug: slug, Name: name, Country: "CZ", DefaultVatRateBps: 2100,
		AttendanceRoundingMin: 15, AttendanceRoundingMode: model.RoundNearest,
		PlanHorizonWeeks: 8, QuoteValidityDays: 30,
	}
	if err := tx.Create(&acc).Error; err != nil {
		return nil, dbErr(err, "account")
	}
	if err := tx.Create(&model.Membership{UserID: ownerID, AccountID: acc.ID, Role: model.RoleOwner}).Error; err != nil {
		return nil, dbErr(err, "membership")
	}
	return &acc, nil
}

func (s *server) registerAccounts(authed, account huma.API) {
	huma.Get(authed, "/api/accounts", s.listAccounts)
	huma.Post(authed, "/api/accounts", s.createAccount, status(http.StatusCreated))
	huma.Get(account, "", s.getAccount, auth.ForEveryone)
	huma.Patch(account, "", s.patchAccount, auth.ForManagers)
}

// accountRow is an account joined with the current user's membership role.
type accountRow struct {
	model.Account
	Role string
}

func (s *server) listAccounts(ctx context.Context, in *struct{ PageParams }) (*Out[ListResponse[Account]], error) {
	q := s.db.WithContext(ctx).Table("accounts").
		Select("accounts.*, memberships.role").
		Joins("JOIN memberships ON memberships.account_id = accounts.id").
		Where("memberships.user_id = ?", auth.UserFrom(ctx).ID).
		Order("accounts.name")
	return paginate(q, in.PageParams, func(r *accountRow) Account { return toAccount(&r.Account, r.Role) })
}

func (s *server) createAccount(ctx context.Context, in *struct{ Body AccountCreate }) (*Out[Account], error) {
	var acc *model.Account
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) (err error) {
		acc, err = newAccount(tx, in.Body.Name, auth.UserFrom(ctx).ID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return &Out[Account]{Body: toAccount(acc, model.RoleOwner)}, nil
}

func (s *server) getAccount(ctx context.Context, _ *struct{}) (*Out[Account], error) {
	return &Out[Account]{Body: toAccount(auth.AccountFrom(ctx), auth.RoleFrom(ctx))}, nil
}

func (s *server) patchAccount(ctx context.Context, in *struct{ Body AccountPatch }) (*Out[Account], error) {
	acc := *auth.AccountFrom(ctx)
	p := in.Body
	if p.Name != nil {
		trimmed := strings.TrimSpace(*p.Name)
		p.Name = &trimmed
	}
	apply(&acc.Name, p.Name)
	apply(&acc.RegistrationNo, p.RegistrationNo)
	apply(&acc.VatNo, p.VatNo)
	apply(&acc.Street, p.Street)
	apply(&acc.City, p.City)
	apply(&acc.Zip, p.Zip)
	apply(&acc.Country, p.Country)
	apply(&acc.Email, p.Email)
	apply(&acc.Phone, p.Phone)
	apply(&acc.Web, p.Web)
	apply(&acc.VatPayer, p.VatPayer)
	apply(&acc.DefaultVatRateBps, p.DefaultVatRateBps)
	apply(&acc.DefaultHourlyRate, p.DefaultHourlyRate)
	apply(&acc.DefaultKmRate, p.DefaultKmRate)
	apply(&acc.AttendanceRoundingMin, p.AttendanceRoundingMin)
	apply(&acc.AttendanceRoundingMode, p.AttendanceRoundingMode)
	apply(&acc.AttendanceGPS, p.AttendanceGPS)
	apply(&acc.PlanHorizonWeeks, p.PlanHorizonWeeks)
	apply(&acc.QuoteValidityDays, p.QuoteValidityDays)
	apply(&acc.QuoteTerms, p.QuoteTerms)
	if p.TimeZone != nil {
		tz := strings.TrimSpace(*p.TimeZone)
		if tz != "" {
			if _, err := time.LoadLocation(tz); err != nil {
				return nil, invalid("time_zone", "unknown time zone")
			}
		}
		acc.TimeZone = tz
	}
	if p.Onboarded != nil {
		switch {
		case !*p.Onboarded:
			acc.OnboardedAt = nil
		case acc.OnboardedAt == nil:
			now := s.deps.Now()
			acc.OnboardedAt = &now
		}
	}
	if acc.Name == "" {
		return nil, invalid("name", "name must not be empty")
	}
	for _, img := range []struct {
		field string
		src   *uint
		dst   **uint
	}{{"logo_attachment_id", p.LogoAttachmentID, &acc.LogoAttachmentID}} {
		if img.src == nil {
			continue
		}
		if *img.src == 0 {
			*img.dst = nil
			continue
		}
		if err := s.checkImageAttachment(ctx, img.field, *img.src); err != nil {
			return nil, err
		}
		id := *img.src
		*img.dst = &id
	}
	if err := s.db.WithContext(ctx).Save(&acc).Error; err != nil {
		return nil, dbErr(err, "account")
	}
	return &Out[Account]{Body: toAccount(&acc, auth.RoleFrom(ctx))}, nil
}

// Image limits of the logo (it is embedded into PDFs).
const (
	maxImageBytes = 2 << 20
	maxImageSide  = 4000
)

// checkImageAttachment: the attachment must belong to the account and be a
// PNG or JPEG of sane size → 422 on field otherwise.
func (s *server) checkImageAttachment(ctx context.Context, field string, id uint) error {
	var a model.Attachment
	if err := s.scoped(ctx).First(&a, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return invalid(field, "attachment not found")
		}
		return dbErr(err, "attachment")
	}
	if a.ContentType != "image/png" && a.ContentType != "image/jpeg" {
		return invalid(field, "attachment must be a PNG or JPEG image")
	}
	if a.Size > maxImageBytes {
		return invalid(field, fmt.Sprintf("image must be at most %d MB", maxImageBytes>>20))
	}
	b := s.attachmentBytes(ctx, &id)
	if b == nil {
		return invalid(field, "attachment cannot be read")
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(b))
	if err != nil || cfg.Width > maxImageSide || cfg.Height > maxImageSide {
		return invalid(field, fmt.Sprintf("image must be a valid PNG or JPEG of at most %d×%d px", maxImageSide, maxImageSide))
	}
	return nil
}
