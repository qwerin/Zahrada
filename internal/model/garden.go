package model

import (
	"time"

	"gorm.io/gorm"

	"github.com/qwerin/zahrada/internal/search"
)

// Client is a customer of the firm (SPEC §4.2).
type Client struct {
	ID             uint
	AccountID      uint   `gorm:"not null;index"`
	Name           string `gorm:"not null"`
	Kind           string `gorm:"not null;default:person"` // person | company
	RegistrationNo string // IČO
	VatNo          string
	Email          string
	Phone          string
	Street         string
	City           string
	Zip            string
	Note           string
	Archived       bool `gorm:"not null;default:false"`
	NFSubjectID    *uint
	SearchText     string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// Site is a garden (place of work) of a client (SPEC §4.3).
type Site struct {
	ID         uint
	AccountID  uint   `gorm:"not null;index"`
	ClientID   uint   `gorm:"not null;index"`
	Name       string `gorm:"not null"`
	Street     string
	City       string
	Zip        string
	Lat        *float64
	Lng        *float64
	AreaM2     *int64
	LawnM2     *int64
	HedgeBM    *int64 `gorm:"column:hedge_bm"`
	AccessNote string // klíč, kód brány, pes — contractors see it for their work
	Note       string
	Archived   bool `gorm:"not null;default:false"`
	SearchText string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// Contractor works for the firm and records attendance (SPEC §4.4).
type Contractor struct {
	ID             uint
	AccountID      uint   `gorm:"not null;index"`
	Name           string `gorm:"not null"`
	Kind           string `gorm:"not null;default:person"` // person | company
	RegistrationNo string
	Phone          string
	Email          string
	HourlyRate     int64  `gorm:"not null;default:0"` // haléře/h
	KmRate         int64  `gorm:"not null;default:0"` // haléře/km
	Color          string // "#RRGGBB" in the plan
	Active         bool   `gorm:"not null;default:true"`
	UserID         *uint  `gorm:"index"` // login with role contractor
	NFSubjectID    *uint
	Note           string
	SearchText     string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// Job kinds and statuses (SPEC §4.7).
const (
	JobRealization = "realization"
	JobMaintenance = "maintenance"
	JobOneOff      = "one_off"

	JobInquiry    = "inquiry"
	JobQuoted     = "quoted"
	JobOrdered    = "ordered"
	JobInProgress = "in_progress"
	JobDone       = "done"
	JobInvoiced   = "invoiced"
	JobCancelled  = "cancelled"
)

// Job is a zakázka (SPEC §4.7).
type Job struct {
	ID          uint
	AccountID   uint   `gorm:"not null;index;uniqueIndex:idx_jobs_account_number"`
	Number      string `gorm:"not null;uniqueIndex:idx_jobs_account_number"`
	ClientID    uint   `gorm:"not null;index"`
	SiteID      *uint  `gorm:"index"`
	Kind        string `gorm:"not null"`
	Title       string `gorm:"not null"`
	Status      string `gorm:"not null;index"`
	PlannedFrom string // YYYY-MM-DD
	PlannedTo   string
	MonthlyFee  *int64
	Note        string
	// AcceptedQuoteID is the accepted offer (quote version) of the job.
	AcceptedQuoteID *uint
	SearchText      string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// JobContractor assigns a contractor to a job (they see it under /me and may record attendance on it).
type JobContractor struct {
	JobID        uint `gorm:"primaryKey"`
	ContractorID uint `gorm:"primaryKey;index"`
	AccountID    uint `gorm:"not null;index"`
}

// Attendance statuses and sources (SPEC §4.8, internal/attendance).
const (
	AttendanceDraft     = "draft"
	AttendanceSubmitted = "submitted"
	AttendanceApproved  = "approved"
	AttendanceRejected  = "rejected"

	SourceManual = "manual"
	SourceClock  = "clock"
)

// AttendanceEntry is one span of work of one contractor on one day (SPEC §4.8).
// Times are minutes from local midnight in the account's zone; EndMin nil = open (clocked in).
type AttendanceEntry struct {
	ID             uint
	AccountID      uint   `gorm:"not null;index:idx_att_account_on"`
	ContractorID   uint   `gorm:"not null;index:idx_att_contractor_on"`
	JobID          *uint  `gorm:"index"` // nil = režie
	WorkOn         string `gorm:"not null;index:idx_att_account_on;index:idx_att_contractor_on"`
	StartMin       int    `gorm:"not null"`
	EndMin         *int
	BreakMin       int   `gorm:"not null;default:0"`
	KM             int64 `gorm:"column:km;not null;default:0"`
	Note           string
	Status         string `gorm:"not null;index"`
	Source         string `gorm:"not null"`
	RateSnapshot   int64  `gorm:"not null"`
	KmRateSnapshot int64  `gorm:"not null"`
	CreatedBy      *uint
	ApprovedBy     *uint
	ApprovedAt     *time.Time
	RejectReason   string
	LatIn, LngIn   *float64
	LatOut, LngOut *float64
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (m *Client) SearchSource() []string {
	return []string{m.Name, m.RegistrationNo, m.Email, m.Phone, m.City, m.Street}
}
func (m *Site) SearchSource() []string { return []string{m.Name, m.Street, m.City} }
func (m *Contractor) SearchSource() []string {
	return []string{m.Name, m.RegistrationNo, m.Email, m.Phone}
}
func (m *Job) SearchSource() []string { return []string{m.Number, m.Title} }

func (m *Client) BeforeSave(*gorm.DB) error {
	m.SearchText = search.Text(m.SearchSource()...)
	return nil
}
func (m *Site) BeforeSave(*gorm.DB) error {
	m.SearchText = search.Text(m.SearchSource()...)
	return nil
}
func (m *Contractor) BeforeSave(*gorm.DB) error {
	m.SearchText = search.Text(m.SearchSource()...)
	return nil
}
func (m *Job) BeforeSave(*gorm.DB) error {
	m.SearchText = search.Text(m.SearchSource()...)
	return nil
}
