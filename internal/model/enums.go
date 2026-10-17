package model

// Membership roles (SPEC §3). Permissions per role: see auth.Allow.
const (
	RoleOwner      = "owner"      // everything
	RoleAdmin      = "admin"      // everything except managing owners (and deleting the account)
	RoleAccountant = "accountant" // read-only + exports/reports
	RoleMember     = "member"     // vedoucí: clients, sites, quotes, jobs, plan, attendance approval; no settings
	RoleContractor = "contractor" // dodavatel: only /me endpoints (own visits and attendance)
)

// Rounding modes of attendance (Account.AttendanceRoundingMode, internal/attendance).
const (
	RoundNearest = "nearest"
	RoundUp      = "up"
	RoundDown    = "down"
)
