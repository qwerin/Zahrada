package model

import (
	"time"

	"gorm.io/gorm"

	"github.com/qwerin/zahrada/internal/search"
)

// PriceItem is a line of the price list (ceník), copied into quotes (SPEC §4.5).
type PriceItem struct {
	ID             uint
	AccountID      uint   `gorm:"not null;index"`
	Kind           string `gorm:"not null"` // calc.Kind
	Code           string
	Name           string `gorm:"not null"`
	Unit           string
	Category       string
	UnitCost       int64  `gorm:"not null;default:0"`
	UnitPrice      *int64 // nil = UnitCost + MarkupBps
	MarkupBps      int32  `gorm:"not null;default:0"`
	WasteBps       int32  `gorm:"not null;default:0"`
	NormHoursMilli int64  `gorm:"not null;default:0"`
	VatRateBps     int32  `gorm:"not null;default:2100"`
	Archived       bool   `gorm:"not null;default:false"`
	SearchText     string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (m *PriceItem) SearchSource() []string { return []string{m.Code, m.Name, m.Category} }
func (m *PriceItem) BeforeSave(*gorm.DB) error {
	m.SearchText = search.Text(m.SearchSource()...)
	return nil
}

// Quote statuses (SPEC §4.6).
const (
	QuoteDraft      = "draft"
	QuoteSent       = "sent"
	QuoteAccepted   = "accepted"
	QuoteRejected   = "rejected"
	QuoteSuperseded = "superseded"
)

// Quote is one version of the calculation / offer of a job (SPEC §4.6).
// Totals are recomputed by calc.Compute on every save.
type Quote struct {
	ID          uint
	AccountID   uint   `gorm:"not null;index"`
	JobID       uint   `gorm:"not null;uniqueIndex:idx_quotes_job_version"`
	Version     int    `gorm:"not null;uniqueIndex:idx_quotes_job_version"`
	Title       string `gorm:"not null"`
	Status      string `gorm:"not null"`
	ValidUntil  string
	DiscountBps int32  `gorm:"not null;default:0"`
	Intro       string // text for the client above the items
	Note        string // text for the client below the totals
	Net         int64
	VATTotal    int64 `gorm:"column:vat_total"`
	Gross       int64
	Cost        int64
	Margin      int64
	MarginBps   int32
	HoursMilli  int64
	SentAt      *time.Time
	AcceptedAt  *time.Time
	Items       []QuoteItem `gorm:"constraint:OnDelete:CASCADE"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// QuoteItem is one line of a quote (values copied, not linked to the price list).
type QuoteItem struct {
	ID             uint
	QuoteID        uint `gorm:"not null;index"`
	AccountID      uint `gorm:"not null;index"`
	Position       int  `gorm:"not null"`
	Section        string
	Kind           string `gorm:"not null"`
	PriceItemID    *uint
	Name           string `gorm:"not null"`
	Unit           string
	QuantityMilli  int64 `gorm:"not null"`
	WasteBps       int32
	UnitCost       int64
	UnitPrice      *int64
	MarkupBps      int32
	DiscountBps    int32
	VatRateBps     int32
	NormHoursMilli int64
	Note           string
}
