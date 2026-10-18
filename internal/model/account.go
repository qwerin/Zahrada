package model

import "time"

// Account is a company profile with the settings of SPEC §4.1.
type Account struct {
	ID             uint
	Slug           string `gorm:"not null;uniqueIndex"`
	Name           string `gorm:"not null"`
	RegistrationNo string
	VatNo          string
	Street         string
	City           string
	Zip            string
	Country        string
	Email          string
	Phone          string
	Web            string

	VatPayer          bool
	DefaultVatRateBps int32
	DefaultHourlyRate int64 // haléře/h — default rate of a new contractor
	DefaultKmRate     int64 // haléře/km

	AttendanceRoundingMin   int    // 0 = none, 5, 10, 15, 30
	AttendanceRoundingMode  string // nearest | up | down
	AttendanceLockedThrough string // "YYYY-MM-DD" or "" — attendance closed through this day
	AttendanceGPS           bool

	PlanHorizonWeeks  int // how far ahead maintenance visits are generated
	QuoteValidityDays int
	QuoteTerms        string // terms printed on quote PDFs
	TimeZone          string // "" = ZAHRADA_TIMEZONE

	LogoAttachmentID *uint // image Attachment of this account, used in PDFs
	OnboardedAt      *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}
