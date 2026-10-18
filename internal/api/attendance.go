package api

// Attendance of contractors — SPEC §4.8. The rules (validation, overlap,
// rounding, status transitions, closing, summary) live in internal/attendance;
// this file maps DTOs and storage onto them.

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"gorm.io/gorm"

	"github.com/qwerin/zahrada/internal/attendance"
	"github.com/qwerin/zahrada/internal/auth"
	"github.com/qwerin/zahrada/internal/events"
	"github.com/qwerin/zahrada/internal/model"
)

// ---- DTOs ----

type AttendanceEntry struct {
	ID             uint       `json:"id"`
	ContractorID   uint       `json:"contractor_id"`
	ContractorName string     `json:"contractor_name"`
	JobID          *uint      `json:"job_id,omitempty" doc:"Missing = overhead (režie)"`
	JobNumber      string     `json:"job_number,omitempty"`
	JobTitle       string     `json:"job_title,omitempty"`
	WorkOn         string     `json:"work_on" format:"date"`
	Start          string     `json:"start" example:"07:30"`
	End            *string    `json:"end,omitempty" example:"15:45" doc:"Missing = still clocked in"`
	BreakMin       int        `json:"break_min"`
	KM             int64      `json:"km"`
	Note           string     `json:"note"`
	Status         string     `json:"status" enum:"draft,submitted,approved,rejected"`
	Source         string     `json:"source" enum:"manual,clock"`
	WorkedMin      int        `json:"worked_min" doc:"Worked minutes after the account's rounding (0 while open)"`
	Cost           int64      `json:"cost" doc:"Labour cost at the snapshot rate, haléře"`
	KmCost         int64      `json:"km_cost"`
	RateSnapshot   int64      `json:"rate_snapshot"`
	KmRateSnapshot int64      `json:"km_rate_snapshot"`
	RejectReason   string     `json:"reject_reason,omitempty"`
	ApprovedAt     *time.Time `json:"approved_at,omitempty"`
	Editable       bool       `json:"editable" doc:"The current user may change or delete it"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// AttendanceInput is shared by staff create/patch (pointers = optional on patch).
type AttendanceInput struct {
	ContractorID   *uint   `json:"contractor_id,omitempty" doc:"Staff only; required on create"`
	JobID          *uint   `json:"job_id,omitempty" doc:"0 = overhead (režie)"`
	WorkOn         *string `json:"work_on,omitempty" format:"date"`
	Start          *string `json:"start,omitempty" example:"07:30"`
	End            *string `json:"end,omitempty" example:"15:45"`
	BreakMin       *int    `json:"break_min,omitempty" minimum:"0" maximum:"1440"`
	KM             *int64  `json:"km,omitempty" minimum:"0" maximum:"5000"`
	Note           *string `json:"note,omitempty" maxLength:"2000"`
	Status         *string `json:"status,omitempty" enum:"draft,submitted,approved" doc:"Staff only; create defaults to approved"`
	RateSnapshot   *int64  `json:"rate_snapshot,omitempty" minimum:"0" doc:"Staff only; default = contractor's current rate"`
	KmRateSnapshot *int64  `json:"km_rate_snapshot,omitempty" minimum:"0"`
}

type AttendanceIDs struct {
	IDs    []uint `json:"ids" minItems:"1" maxItems:"500"`
	Reason string `json:"reason,omitempty" maxLength:"500" doc:"Reject only"`
}

type AttendanceJobLine struct {
	JobID     uint   `json:"job_id" doc:"0 = overhead"`
	JobNumber string `json:"job_number,omitempty"`
	JobTitle  string `json:"job_title,omitempty"`
	Minutes   int    `json:"minutes"`
	Cost      int64  `json:"cost"`
	KM        int64  `json:"km"`
	KmCost    int64  `json:"km_cost"`
}

type AttendanceContractorSummary struct {
	ContractorID   uint                `json:"contractor_id"`
	ContractorName string              `json:"contractor_name"`
	Days           int                 `json:"days"`
	Minutes        int                 `json:"minutes"`
	Cost           int64               `json:"cost"`
	KM             int64               `json:"km"`
	KmCost         int64               `json:"km_cost"`
	Total          int64               `json:"total"`
	Jobs           []AttendanceJobLine `json:"jobs" nullable:"false"`
}

type AttendanceSummary struct {
	Month          string                        `json:"month" example:"2026-05"`
	From           string                        `json:"from" format:"date"`
	To             string                        `json:"to" format:"date"`
	Locked         bool                          `json:"locked" doc:"The whole month is closed"`
	LockedThrough  string                        `json:"locked_through"`
	PendingCount   int64                         `json:"pending_count" doc:"Entries of the month not approved yet (not in the totals)"`
	Contractors    []AttendanceContractorSummary `json:"contractors" nullable:"false"`
	TotalMinutes   int                           `json:"total_minutes"`
	TotalCost      int64                         `json:"total_cost"`
	RoundingMin    int                           `json:"rounding_min"`
	RoundingMode   string                        `json:"rounding_mode"`
	ApprovedOnlyOK bool                          `json:"approved_only" doc:"Always true: totals contain approved entries only"`
}

type AttendanceClose struct {
	Through string `json:"through" format:"date" doc:"Last closed day; an earlier date reopens"`
}

type MeContractor struct {
	ContractorID uint             `json:"contractor_id"`
	Name         string           `json:"name"`
	Today        string           `json:"today" format:"date" doc:"Today in the account's time zone"`
	Now          string           `json:"now" example:"07:31"`
	Open         *AttendanceEntry `json:"open,omitempty" doc:"Current clock-in, if any"`
}

type MeJob struct {
	ID          uint   `json:"id"`
	Number      string `json:"number"`
	Title       string `json:"title"`
	Status      string `json:"status"`
	ClientName  string `json:"client_name"`
	SiteName    string `json:"site_name,omitempty"`
	Address     string `json:"address,omitempty"`
	AccessNote  string `json:"access_note,omitempty"`
	PlannedFrom string `json:"planned_from,omitempty"`
	PlannedTo   string `json:"planned_to,omitempty"`
	Note        string `json:"note"`
}

type ClockIn struct {
	JobID *uint    `json:"job_id,omitempty" doc:"An assigned job; omitted = overhead"`
	Lat   *float64 `json:"lat,omitempty" minimum:"-90" maximum:"90" doc:"Stored only when the account enables GPS"`
	Lng   *float64 `json:"lng,omitempty" minimum:"-180" maximum:"180"`
}

type ClockOut struct {
	BreakMin int      `json:"break_min,omitempty" minimum:"0" maximum:"1440"`
	KM       int64    `json:"km,omitempty" minimum:"0" maximum:"5000"`
	Note     string   `json:"note,omitempty" maxLength:"2000"`
	Lat      *float64 `json:"lat,omitempty" minimum:"-90" maximum:"90"`
	Lng      *float64 `json:"lng,omitempty" minimum:"-180" maximum:"180"`
}

type CSVOut struct {
	ContentType        string `header:"Content-Type"`
	ContentDisposition string `header:"Content-Disposition"`
	Body               []byte
}

// ---- routes ----

func (s *server) registerAttendance(g huma.API) {
	huma.Get(g, "/attendance", s.listAttendance)
	huma.Post(g, "/attendance", s.createAttendance, status(http.StatusCreated), auth.ForEditors)
	huma.Patch(g, "/attendance/{id}", s.patchAttendance, auth.ForEditors)
	huma.Delete(g, "/attendance/{id}", s.deleteAttendance, status(http.StatusNoContent), auth.ForEditors)
	huma.Post(g, "/attendance/actions/approve", s.approveAttendance, auth.ForEditors)
	huma.Post(g, "/attendance/actions/reject", s.rejectAttendance, auth.ForEditors)
	huma.Post(g, "/attendance/actions/reopen", s.reopenAttendance, auth.ForEditors)
	huma.Post(g, "/attendance/actions/close", s.closeAttendance, auth.ForManagers)
	huma.Get(g, "/attendance/summary", s.attendanceSummary)
	huma.Get(g, "/attendance/export.csv", s.attendanceCSV)

	// contractor self-service (also staff who are linked to a contractor)
	huma.Get(g, "/me/contractor", s.meContractor, auth.ForEveryone)
	huma.Get(g, "/me/jobs", s.meJobs, auth.ForEveryone)
	huma.Get(g, "/me/attendance", s.meAttendance, auth.ForEveryone)
	huma.Post(g, "/me/attendance", s.meCreateAttendance, status(http.StatusCreated), auth.ForEveryone)
	huma.Patch(g, "/me/attendance/{id}", s.mePatchAttendance, auth.ForEveryone)
	huma.Delete(g, "/me/attendance/{id}", s.meDeleteAttendance, status(http.StatusNoContent), auth.ForEveryone)
	huma.Post(g, "/me/attendance/clock-in", s.clockIn, status(http.StatusCreated), auth.ForEveryone)
	huma.Post(g, "/me/attendance/clock-out", s.clockOut, auth.ForEveryone)
	huma.Post(g, "/me/attendance/actions/submit", s.meSubmit, auth.ForEveryone)
}

// ---- helpers ----

func (s *server) rounding(ctx context.Context) attendance.Rounding {
	a := auth.AccountFrom(ctx)
	return attendance.Rounding{StepMin: a.AttendanceRoundingMin, Mode: attendance.RoundMode(defaultStr(a.AttendanceRoundingMode, model.RoundNearest))}
}

func toDomain(m *model.AttendanceEntry) attendance.Entry {
	e := attendance.Entry{ID: m.ID, ContractorID: m.ContractorID, Date: m.WorkOn, StartMin: m.StartMin, EndMin: 24 * 60,
		BreakMin: m.BreakMin, KM: m.KM, Status: attendance.Status(m.Status), RateSnapshot: m.RateSnapshot, KMRateSnapshot: m.KmRateSnapshot}
	if m.JobID != nil {
		e.JobID = *m.JobID
	}
	if m.EndMin != nil {
		e.EndMin = *m.EndMin // an open entry blocks the rest of its day (overlap)
	}
	return e
}

func (s *server) toAttendance(ctx context.Context, m *model.AttendanceEntry, manager bool) AttendanceEntry {
	out := AttendanceEntry{ID: m.ID, ContractorID: m.ContractorID, JobID: m.JobID, WorkOn: m.WorkOn,
		Start: attendance.FormatClock(m.StartMin), BreakMin: m.BreakMin, KM: m.KM, Note: m.Note, Status: m.Status,
		Source: m.Source, RateSnapshot: m.RateSnapshot, KmRateSnapshot: m.KmRateSnapshot, RejectReason: m.RejectReason,
		ApprovedAt: m.ApprovedAt, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
		KmCost: m.KM * m.KmRateSnapshot}
	if m.EndMin != nil {
		end := attendance.FormatClock(*m.EndMin)
		out.End = &end
		out.WorkedMin = s.rounding(ctx).Apply(toDomain(m).WorkedMinutes())
		out.Cost = attendance.Cost(out.WorkedMin, m.RateSnapshot)
	}
	out.Editable = attendance.Editable(toDomain(m), manager, auth.AccountFrom(ctx).AttendanceLockedThrough)
	return out
}

// fillAttendance adds contractor and job names.
func (s *server) fillAttendance(ctx context.Context, items []AttendanceEntry) {
	var cids, jids []uint
	for _, e := range items {
		cids = append(cids, e.ContractorID)
		if e.JobID != nil {
			jids = append(jids, *e.JobID)
		}
	}
	names := s.namesOf(ctx, s.db.WithContext(ctx), &model.Contractor{}, cids)
	jobs := map[uint]model.Job{}
	if len(jids) > 0 {
		var rows []model.Job
		if err := s.scoped(ctx).Select("id", "number", "title").Where("id IN ?", jids).Find(&rows).Error; err == nil {
			for _, j := range rows {
				jobs[j.ID] = j
			}
		}
	}
	for i := range items {
		items[i].ContractorName = names[items[i].ContractorID]
		if items[i].JobID != nil {
			items[i].JobNumber, items[i].JobTitle = jobs[*items[i].JobID].Number, jobs[*items[i].JobID].Title
		}
	}
}

func (s *server) attendanceOut(ctx context.Context, m *model.AttendanceEntry, manager bool) AttendanceEntry {
	out := []AttendanceEntry{s.toAttendance(ctx, m, manager)}
	s.fillAttendance(ctx, out)
	return out[0]
}

// validateEntry runs the domain rules on m (closed m.EndMin required unless open)
// and checks overlaps with the contractor's other entries that day — inside tx.
func (s *server) validateEntry(ctx context.Context, tx *gorm.DB, m *model.AttendanceEntry) error {
	locked := auth.AccountFrom(ctx).AttendanceLockedThrough
	if attendance.Locked(m.WorkOn, locked) {
		return conflict(CodePeriodClosed, "attendance is closed through "+locked)
	}
	e := toDomain(m)
	if m.EndMin == nil {
		e.EndMin = min(m.StartMin+1, 24*60) // open entry: only the start is known
		if m.StartMin >= 24*60 {
			e.EndMin = m.StartMin
		}
	}
	if err := e.Validate(); err != nil {
		field := "end"
		switch {
		case errors.Is(err, attendance.ErrDate):
			field = "work_on"
		case errors.Is(err, attendance.ErrBreak):
			field = "break_min"
		case errors.Is(err, attendance.ErrKM):
			field = "km"
		}
		return invalid(field, err.Error())
	}
	var same []model.AttendanceEntry
	if err := tx.Scopes(inAccount(ctx)).Where("contractor_id = ? AND work_on = ? AND id <> ?", m.ContractorID, m.WorkOn, m.ID).
		Find(&same).Error; err != nil {
		return dbErr(err, "attendance")
	}
	others := make([]attendance.Entry, len(same))
	for i := range same {
		others[i] = toDomain(&same[i])
	}
	if m.EndMin == nil {
		e.EndMin = 24 * 60 // an open entry claims the rest of the day
	}
	if o := attendance.FindOverlap(others, e); o != nil {
		return conflict(CodeAttendanceOverlap, fmt.Sprintf("overlaps another entry of the day (%s–%s)",
			attendance.FormatClock(o.StartMin), attendance.FormatClock(o.EndMin)))
	}
	return nil
}

// applyInput copies an input onto m; staff=false ignores staff-only fields.
func applyAttendanceInput(m *model.AttendanceEntry, in AttendanceInput) error {
	if in.JobID != nil {
		if *in.JobID == 0 {
			m.JobID = nil
		} else {
			id := *in.JobID
			m.JobID = &id
		}
	}
	apply(&m.WorkOn, in.WorkOn)
	if in.Start != nil {
		v, err := attendance.ParseClock(*in.Start)
		if err != nil {
			return invalid("start", "use HH:MM")
		}
		m.StartMin = v
	}
	if in.End != nil {
		if strings.TrimSpace(*in.End) == "" {
			m.EndMin = nil
		} else {
			v, err := attendance.ParseClock(*in.End)
			if err != nil {
				return invalid("end", "use HH:MM")
			}
			m.EndMin = &v
		}
	}
	apply(&m.BreakMin, in.BreakMin)
	apply(&m.KM, in.KM)
	apply(&m.Note, in.Note)
	return nil
}

// checkJobForContractor: the job must exist; contractors may only use jobs assigned to them.
func (s *server) checkJobForContractor(ctx context.Context, tx *gorm.DB, jobID *uint, contractorID uint, staff bool) (*model.Job, error) {
	if jobID == nil {
		return nil, nil
	}
	j, err := s.loadJob(ctx, tx, *jobID)
	if err != nil {
		return nil, invalid("job_id", "job not found")
	}
	if !staff {
		var n int64
		if err := tx.Model(&model.JobContractor{}).Where("job_id = ? AND contractor_id = ?", j.ID, contractorID).Count(&n).Error; err != nil {
			return nil, dbErr(err, "job contractor")
		}
		if n == 0 {
			return nil, conflict(CodeJobNotAssigned, "this job is not assigned to you")
		}
	}
	return j, nil
}

// startJobOnWork moves an ordered job to in_progress with its first attendance.
func startJobOnWork(ctx context.Context, tx *gorm.DB, j *model.Job) error {
	if j != nil && j.Status == model.JobOrdered {
		return setJobStatus(ctx, tx, j, model.JobInProgress)
	}
	return nil
}

func (s *server) loadEntry(ctx context.Context, tx *gorm.DB, id uint) (*model.AttendanceEntry, error) {
	var m model.AttendanceEntry
	if err := tx.Scopes(inAccount(ctx)).First(&m, id).Error; err != nil {
		return nil, dbErr(err, "attendance entry")
	}
	return &m, nil
}

// ---- staff ----

func (s *server) listAttendance(ctx context.Context, in *struct {
	PageParams
	From         string `query:"from" doc:"YYYY-MM-DD inclusive"`
	To           string `query:"to" doc:"YYYY-MM-DD inclusive"`
	ContractorID uint   `query:"contractor_id"`
	JobID        uint   `query:"job_id"`
	Status       string `query:"status" doc:"Comma-separated: draft, submitted, approved, rejected, open"`
}) (*Out[ListResponse[AttendanceEntry]], error) {
	q, err := attendanceQuery(s.scoped(ctx).Model(&model.AttendanceEntry{}), in.From, in.To, in.Status)
	if err != nil {
		return nil, err
	}
	if in.ContractorID != 0 {
		q = q.Where("contractor_id = ?", in.ContractorID)
	}
	if in.JobID != 0 {
		q = q.Where("job_id = ?", in.JobID)
	}
	out, err := paginate(q.Order("work_on DESC, start_min DESC, id DESC"), in.PageParams,
		func(m *model.AttendanceEntry) AttendanceEntry { return s.toAttendance(ctx, m, true) })
	if err != nil {
		return nil, err
	}
	s.fillAttendance(ctx, out.Body.Items)
	return out, nil
}

func attendanceQuery(q *gorm.DB, from, to, statuses string) (*gorm.DB, error) {
	if from != "" {
		if !validDate(from) {
			return nil, invalid("from", "use YYYY-MM-DD")
		}
		q = q.Where("work_on >= ?", from)
	}
	if to != "" {
		if !validDate(to) {
			return nil, invalid("to", "use YYYY-MM-DD")
		}
		q = q.Where("work_on <= ?", to)
	}
	if statuses != "" {
		var conds [][]any
		for _, st := range splitStatuses(statuses) {
			switch st {
			case model.AttendanceDraft, model.AttendanceSubmitted, model.AttendanceApproved, model.AttendanceRejected:
				conds = append(conds, []any{"status = ?", st})
			case "open":
				conds = append(conds, []any{"end_min IS NULL"})
			default:
				return nil, invalid("status", "unknown status "+st)
			}
		}
		q = orConditions(q, conds)
	}
	return q, nil
}

func (s *server) createAttendance(ctx context.Context, in *struct{ Body AttendanceInput }) (*Out[AttendanceEntry], error) {
	b := in.Body
	if b.ContractorID == nil {
		return nil, invalid("contractor_id", "contractor_id is required")
	}
	if b.WorkOn == nil || b.Start == nil || b.End == nil {
		return nil, invalid("end", "work_on, start and end are required")
	}
	var m model.AttendanceEntry
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		c, err := s.loadContractor(ctx, tx, *b.ContractorID)
		if err != nil {
			return invalid("contractor_id", "contractor not found")
		}
		uid := auth.UserFrom(ctx).ID
		m = model.AttendanceEntry{AccountID: c.AccountID, ContractorID: c.ID, Status: model.AttendanceApproved,
			Source: model.SourceManual, RateSnapshot: c.HourlyRate, KmRateSnapshot: c.KmRate, CreatedBy: &uid}
		if err := applyAttendanceInput(&m, b); err != nil {
			return err
		}
		apply(&m.Status, b.Status)
		apply(&m.RateSnapshot, b.RateSnapshot)
		apply(&m.KmRateSnapshot, b.KmRateSnapshot)
		if m.Status == model.AttendanceApproved {
			now := s.deps.Now()
			m.ApprovedBy, m.ApprovedAt = &uid, &now
		}
		job, err := s.checkJobForContractor(ctx, tx, m.JobID, c.ID, true)
		if err != nil {
			return err
		}
		if err := s.validateEntry(ctx, tx, &m); err != nil {
			return err
		}
		if err := tx.Create(&m).Error; err != nil {
			return dbErr(err, "attendance entry")
		}
		return startJobOnWork(ctx, tx, job)
	})
	if err != nil {
		return nil, err
	}
	return &Out[AttendanceEntry]{Body: s.attendanceOut(ctx, &m, true)}, nil
}

func (s *server) patchAttendance(ctx context.Context, in *struct {
	ID   uint `path:"id"`
	Body AttendanceInput
}) (*Out[AttendanceEntry], error) {
	var m *model.AttendanceEntry
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) (err error) {
		if m, err = s.loadEntry(ctx, tx, in.ID); err != nil {
			return err
		}
		if !attendance.Editable(toDomain(m), true, auth.AccountFrom(ctx).AttendanceLockedThrough) {
			return conflict(CodePeriodClosed, "attendance of this day is closed")
		}
		b := in.Body
		if b.ContractorID != nil && *b.ContractorID != m.ContractorID {
			c, err := s.loadContractor(ctx, tx, *b.ContractorID)
			if err != nil {
				return invalid("contractor_id", "contractor not found")
			}
			m.ContractorID, m.RateSnapshot, m.KmRateSnapshot = c.ID, c.HourlyRate, c.KmRate
		}
		if err := applyAttendanceInput(m, b); err != nil {
			return err
		}
		apply(&m.RateSnapshot, b.RateSnapshot)
		apply(&m.KmRateSnapshot, b.KmRateSnapshot)
		if b.Status != nil && *b.Status != m.Status {
			if err := attendance.Transition(attendance.Status(m.Status), attendance.Status(*b.Status), true); err != nil {
				return conflict(CodeInvalidTransition, err.Error())
			}
			s.setAttendanceStatus(ctx, m, *b.Status, "")
		}
		job, err := s.checkJobForContractor(ctx, tx, m.JobID, m.ContractorID, true)
		if err != nil {
			return err
		}
		if err := s.validateEntry(ctx, tx, m); err != nil {
			return err
		}
		if err := tx.Save(m).Error; err != nil {
			return dbErr(err, "attendance entry")
		}
		return startJobOnWork(ctx, tx, job)
	})
	if err != nil {
		return nil, err
	}
	return &Out[AttendanceEntry]{Body: s.attendanceOut(ctx, m, true)}, nil
}

func (s *server) deleteAttendance(ctx context.Context, in *idPath) (*NoContent, error) {
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		m, err := s.loadEntry(ctx, tx, in.ID)
		if err != nil {
			return err
		}
		if !attendance.Editable(toDomain(m), true, auth.AccountFrom(ctx).AttendanceLockedThrough) {
			return conflict(CodePeriodClosed, "attendance of this day is closed")
		}
		return dbErrOrNil(tx.Delete(m).Error, "attendance entry")
	})
	if err != nil {
		return nil, err
	}
	return &NoContent{}, nil
}

// setAttendanceStatus sets status and the approval fields.
func (s *server) setAttendanceStatus(ctx context.Context, m *model.AttendanceEntry, to, reason string) {
	m.Status = to
	switch to {
	case model.AttendanceApproved:
		uid, now := auth.UserFrom(ctx).ID, s.deps.Now()
		m.ApprovedBy, m.ApprovedAt, m.RejectReason = &uid, &now, ""
	case model.AttendanceRejected:
		m.ApprovedBy, m.ApprovedAt, m.RejectReason = nil, nil, reason
	default:
		m.ApprovedBy, m.ApprovedAt = nil, nil
	}
}

// transitionMany changes the status of entries atomically (all or none).
func (s *server) transitionMany(ctx context.Context, ids []uint, to, reason, event, verb string, manager bool, contractorID *uint) (*Out[ListResponse[AttendanceEntry]], error) {
	ids = slices.Compact(slices.Sorted(slices.Values(ids)))
	var rows []model.AttendanceEntry
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		q := tx.Scopes(inAccount(ctx)).Where("id IN ?", ids)
		if contractorID != nil {
			q = q.Where("contractor_id = ?", *contractorID)
		}
		if err := q.Order("work_on, start_min").Find(&rows).Error; err != nil {
			return dbErr(err, "attendance entry")
		}
		if len(rows) != len(ids) {
			return notFound("attendance entry")
		}
		locked := auth.AccountFrom(ctx).AttendanceLockedThrough
		for i := range rows {
			m := &rows[i]
			if attendance.Locked(m.WorkOn, locked) {
				return conflict(CodePeriodClosed, "attendance of "+m.WorkOn+" is closed")
			}
			if m.EndMin == nil {
				return conflict(CodeNotClockedIn, "an entry of "+m.WorkOn+" is still open (clock out first)")
			}
			if err := attendance.Transition(attendance.Status(m.Status), attendance.Status(to), manager); err != nil {
				return conflict(CodeInvalidTransition, fmt.Sprintf("entry %d: %v", m.ID, err))
			}
			s.setAttendanceStatus(ctx, m, to, reason)
			if err := tx.Save(m).Error; err != nil {
				return dbErr(err, "attendance entry")
			}
		}
		return recordSimple(ctx, tx, event, events.SubjectAttendance, rows[0].ID,
			fmt.Sprintf("Docházka %s: %d záznamů", verb, len(rows)), map[string]any{"ids": ids})
	})
	if err != nil {
		return nil, err
	}
	items := make([]AttendanceEntry, len(rows))
	for i := range rows {
		items[i] = s.toAttendance(ctx, &rows[i], manager)
	}
	s.fillAttendance(ctx, items)
	return &Out[ListResponse[AttendanceEntry]]{Body: ListResponse[AttendanceEntry]{Items: items, Page: 1, PerPage: len(items), Total: int64(len(items))}}, nil
}

func (s *server) approveAttendance(ctx context.Context, in *struct{ Body AttendanceIDs }) (*Out[ListResponse[AttendanceEntry]], error) {
	return s.transitionMany(ctx, in.Body.IDs, model.AttendanceApproved, "", events.AttendanceApproved, "schválena", true, nil)
}

func (s *server) rejectAttendance(ctx context.Context, in *struct{ Body AttendanceIDs }) (*Out[ListResponse[AttendanceEntry]], error) {
	return s.transitionMany(ctx, in.Body.IDs, model.AttendanceRejected, strings.TrimSpace(in.Body.Reason), events.AttendanceRejected, "vrácena k opravě", true, nil)
}

func (s *server) reopenAttendance(ctx context.Context, in *struct{ Body AttendanceIDs }) (*Out[ListResponse[AttendanceEntry]], error) {
	return s.transitionMany(ctx, in.Body.IDs, model.AttendanceSubmitted, "", events.AttendanceSubmitted, "znovu otevřena", true, nil)
}

func (s *server) closeAttendance(ctx context.Context, in *struct{ Body AttendanceClose }) (*Out[Account], error) {
	through := in.Body.Through
	if !validDate(through) {
		return nil, invalid("through", "use YYYY-MM-DD")
	}
	acc := *auth.AccountFrom(ctx)
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if through > acc.AttendanceLockedThrough { // closing more days: everything there must be approved
			var n int64
			if err := tx.Model(&model.AttendanceEntry{}).Scopes(inAccount(ctx)).
				Where("work_on <= ? AND (status <> ? OR end_min IS NULL)", through, model.AttendanceApproved).Count(&n).Error; err != nil {
				return dbErr(err, "attendance")
			}
			if n > 0 {
				return conflict(CodePendingEntries, fmt.Sprintf("%d entries up to %s are not approved yet", n, through))
			}
		}
		acc.AttendanceLockedThrough = through
		if err := tx.Model(&acc).Update("attendance_locked_through", through).Error; err != nil {
			return dbErr(err, "account")
		}
		return recordSimple(ctx, tx, events.AttendanceClosed, events.SubjectAccount, acc.ID,
			"Docházka uzavřena do "+czDate(through), map[string]any{"through": through})
	})
	if err != nil {
		return nil, err
	}
	return &Out[Account]{Body: toAccount(&acc, auth.RoleFrom(ctx))}, nil
}

// monthRange parses "YYYY-MM" into its first and last day.
func monthRange(month string) (string, string, error) {
	t, err := time.Parse("2006-01", month)
	if err != nil {
		return "", "", invalid("month", "use YYYY-MM")
	}
	return t.Format(time.DateOnly), t.AddDate(0, 1, -1).Format(time.DateOnly), nil
}

func (s *server) buildSummary(ctx context.Context, month string, contractorID uint) (AttendanceSummary, error) {
	from, to, err := monthRange(month)
	if err != nil {
		return AttendanceSummary{}, err
	}
	acc := auth.AccountFrom(ctx)
	r := s.rounding(ctx)
	out := AttendanceSummary{Month: month, From: from, To: to, LockedThrough: acc.AttendanceLockedThrough,
		Locked: attendance.Locked(to, acc.AttendanceLockedThrough), Contractors: []AttendanceContractorSummary{},
		RoundingMin: r.StepMin, RoundingMode: string(r.Mode), ApprovedOnlyOK: true}
	base := s.scoped(ctx).Model(&model.AttendanceEntry{}).Where("work_on BETWEEN ? AND ?", from, to)
	if contractorID != 0 {
		base = base.Where("contractor_id = ?", contractorID)
	}
	var rows []model.AttendanceEntry
	if err := base.Session(&gorm.Session{}).Where("status = ? AND end_min IS NOT NULL", model.AttendanceApproved).Find(&rows).Error; err != nil {
		return out, dbErr(err, "attendance")
	}
	if err := base.Session(&gorm.Session{}).Where("status <> ? OR end_min IS NULL", model.AttendanceApproved).Count(&out.PendingCount).Error; err != nil {
		return out, dbErr(err, "attendance")
	}
	entries := make([]attendance.Entry, len(rows))
	var cids, jids []uint
	for i := range rows {
		entries[i] = toDomain(&rows[i])
		cids = append(cids, rows[i].ContractorID)
		if rows[i].JobID != nil {
			jids = append(jids, *rows[i].JobID)
		}
	}
	names := s.namesOf(ctx, s.db.WithContext(ctx), &model.Contractor{}, cids)
	jobs := map[uint]model.Job{}
	if len(jids) > 0 {
		var js []model.Job
		if err := s.scoped(ctx).Select("id", "number", "title").Where("id IN ?", jids).Find(&js).Error; err == nil {
			for _, j := range js {
				jobs[j.ID] = j
			}
		}
	}
	for _, c := range attendance.Summarize(entries, r) {
		cs := AttendanceContractorSummary{ContractorID: c.ContractorID, ContractorName: names[c.ContractorID], Days: c.Days,
			Minutes: c.Minutes, Cost: c.Cost, KM: c.KM, KmCost: c.KMCost, Total: c.Total, Jobs: []AttendanceJobLine{}}
		for _, l := range c.Jobs {
			cs.Jobs = append(cs.Jobs, AttendanceJobLine{JobID: l.JobID, JobNumber: jobs[l.JobID].Number, JobTitle: jobs[l.JobID].Title,
				Minutes: l.Minutes, Cost: l.Cost, KM: l.KM, KmCost: l.KMCost})
		}
		out.Contractors = append(out.Contractors, cs)
		out.TotalMinutes += c.Minutes
		out.TotalCost += c.Total
	}
	return out, nil
}

func (s *server) attendanceSummary(ctx context.Context, in *struct {
	Month        string `query:"month" required:"true" example:"2026-05"`
	ContractorID uint   `query:"contractor_id"`
}) (*Out[AttendanceSummary], error) {
	sum, err := s.buildSummary(ctx, in.Month, in.ContractorID)
	if err != nil {
		return nil, err
	}
	return &Out[AttendanceSummary]{Body: sum}, nil
}

// attendanceCSV exports the month's entries (UTF-8 with BOM, semicolons — opens in Excel).
func (s *server) attendanceCSV(ctx context.Context, in *struct {
	Month        string `query:"month" required:"true" example:"2026-05"`
	ContractorID uint   `query:"contractor_id"`
}) (*CSVOut, error) {
	from, to, err := monthRange(in.Month)
	if err != nil {
		return nil, err
	}
	q := s.scoped(ctx).Model(&model.AttendanceEntry{}).Where("work_on BETWEEN ? AND ?", from, to).Order("contractor_id, work_on, start_min")
	if in.ContractorID != 0 {
		q = q.Where("contractor_id = ?", in.ContractorID)
	}
	var rows []model.AttendanceEntry
	if err := q.Find(&rows).Error; err != nil {
		return nil, dbErr(err, "attendance")
	}
	items := make([]AttendanceEntry, len(rows))
	for i := range rows {
		items[i] = s.toAttendance(ctx, &rows[i], true)
	}
	s.fillAttendance(ctx, items)
	var buf bytes.Buffer
	buf.WriteString("\ufeff")
	w := csv.NewWriter(&buf)
	w.Comma = ';'
	_ = w.Write([]string{"Dodavatel", "Datum", "Od", "Do", "Přestávka (min)", "Odpracováno (h)", "Zakázka", "Km", "Cena práce (Kč)", "Cestovné (Kč)", "Stav", "Poznámka"})
	statusCZ := map[string]string{"draft": "rozpracováno", "submitted": "ke schválení", "approved": "schváleno", "rejected": "vráceno"}
	for _, e := range items {
		end := ""
		if e.End != nil {
			end = *e.End
		}
		job := "režie"
		if e.JobID != nil {
			job = strings.TrimSpace(e.JobNumber + " " + e.JobTitle)
		}
		_ = w.Write([]string{csvSafe(e.ContractorName), czDate(e.WorkOn), e.Start, end, fmt.Sprint(e.BreakMin),
			czDecimal(int64(e.WorkedMin)*100/60, 2), csvSafe(job), fmt.Sprint(e.KM), czDecimal(e.Cost, 2), czDecimal(e.KmCost, 2),
			statusCZ[e.Status], csvSafe(e.Note)})
	}
	w.Flush()
	name := fmt.Sprintf("dochazka-%s.csv", in.Month)
	return &CSVOut{ContentType: "text/csv; charset=utf-8", ContentDisposition: `attachment; filename="` + name + `"`, Body: buf.Bytes()}, nil
}

// csvSafe neutralises spreadsheet formulas in user text.
func csvSafe(v string) string {
	if v != "" && strings.ContainsRune("=+-@\t\r", rune(v[0])) {
		return "'" + v
	}
	return v
}

// czDecimal formats v/10^places with a decimal comma (12345, 2 → "123,45").
func czDecimal(v int64, places int) string {
	sign := ""
	if v < 0 {
		sign, v = "-", -v
	}
	div := int64(1)
	for range places {
		div *= 10
	}
	return fmt.Sprintf("%s%d,%0*d", sign, v/div, places, v%div)
}

// ---- contractor self-service (/me) ----

// meContractorOf returns the contractor linked to the current user in this account.
func (s *server) meContractorOf(ctx context.Context, tx *gorm.DB) (*model.Contractor, error) {
	var c model.Contractor
	err := tx.Scopes(inAccount(ctx)).Where("user_id = ?", auth.UserFrom(ctx).ID).First(&c).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, conflict(CodeNotContractor, "your login is not linked to a contractor of this account")
	}
	if err != nil {
		return nil, dbErr(err, "contractor")
	}
	return &c, nil
}

func (s *server) meNow(ctx context.Context) (string, int) {
	return attendance.MinuteOfDay(s.deps.Now(), s.accountLocation(ctx))
}

func (s *server) openEntry(ctx context.Context, tx *gorm.DB, contractorID uint) (*model.AttendanceEntry, error) {
	var m model.AttendanceEntry
	err := tx.Scopes(inAccount(ctx)).Where("contractor_id = ? AND end_min IS NULL", contractorID).Order("work_on DESC, start_min DESC").First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, dbErr(err, "attendance")
	}
	return &m, nil
}

func (s *server) meContractor(ctx context.Context, _ *struct{}) (*Out[MeContractor], error) {
	db := s.db.WithContext(ctx)
	c, err := s.meContractorOf(ctx, db)
	if err != nil {
		return nil, err
	}
	today, minute := s.meNow(ctx)
	out := MeContractor{ContractorID: c.ID, Name: c.Name, Today: today, Now: attendance.FormatClock(minute)}
	open, err := s.openEntry(ctx, db, c.ID)
	if err != nil {
		return nil, err
	}
	if open != nil {
		e := s.attendanceOut(ctx, open, false)
		out.Open = &e
	}
	return &Out[MeContractor]{Body: out}, nil
}

func (s *server) meJobs(ctx context.Context, _ *struct{}) (*Out[[]MeJob], error) {
	db := s.db.WithContext(ctx)
	c, err := s.meContractorOf(ctx, db)
	if err != nil {
		return nil, err
	}
	var jobs []model.Job
	if err := db.Scopes(inAccount(ctx)).
		Where("id IN (?)", db.Model(&model.JobContractor{}).Select("job_id").Where("contractor_id = ?", c.ID)).
		Where("status IN ?", []string{model.JobOrdered, model.JobInProgress}).
		Order("planned_from, id").Find(&jobs).Error; err != nil {
		return nil, dbErr(err, "job")
	}
	var cids, sids []uint
	for _, j := range jobs {
		cids = append(cids, j.ClientID)
		if j.SiteID != nil {
			sids = append(sids, *j.SiteID)
		}
	}
	clients := s.namesOf(ctx, db, &model.Client{}, cids)
	sites := map[uint]model.Site{}
	if len(sids) > 0 {
		var rows []model.Site
		if err := db.Scopes(inAccount(ctx)).Where("id IN ?", sids).Find(&rows).Error; err != nil {
			return nil, dbErr(err, "site")
		}
		for _, s := range rows {
			sites[s.ID] = s
		}
	}
	out := make([]MeJob, len(jobs))
	for i, j := range jobs {
		out[i] = MeJob{ID: j.ID, Number: j.Number, Title: j.Title, Status: j.Status, ClientName: clients[j.ClientID],
			PlannedFrom: j.PlannedFrom, PlannedTo: j.PlannedTo, Note: j.Note}
		if j.SiteID != nil {
			st := sites[*j.SiteID]
			out[i].SiteName, out[i].AccessNote = st.Name, st.AccessNote
			out[i].Address = joinNonEmpty(st.Street, strings.TrimSpace(st.Zip+" "+st.City))
		}
	}
	return &Out[[]MeJob]{Body: out}, nil
}

func (s *server) meAttendance(ctx context.Context, in *struct {
	Month string `query:"month" required:"true" example:"2026-05"`
}) (*Out[ListResponse[AttendanceEntry]], error) {
	from, to, err := monthRange(in.Month)
	if err != nil {
		return nil, err
	}
	db := s.db.WithContext(ctx)
	c, err := s.meContractorOf(ctx, db)
	if err != nil {
		return nil, err
	}
	var rows []model.AttendanceEntry
	if err := db.Scopes(inAccount(ctx)).Where("contractor_id = ? AND work_on BETWEEN ? AND ?", c.ID, from, to).
		Order("work_on DESC, start_min DESC").Find(&rows).Error; err != nil {
		return nil, dbErr(err, "attendance")
	}
	items := make([]AttendanceEntry, len(rows))
	for i := range rows {
		items[i] = s.toAttendance(ctx, &rows[i], false)
	}
	s.fillAttendance(ctx, items)
	return &Out[ListResponse[AttendanceEntry]]{Body: ListResponse[AttendanceEntry]{Items: items, Page: 1, PerPage: len(items), Total: int64(len(items))}}, nil
}

func (s *server) meCreateAttendance(ctx context.Context, in *struct{ Body AttendanceInput }) (*Out[AttendanceEntry], error) {
	b := in.Body
	if b.WorkOn == nil || b.Start == nil || b.End == nil {
		return nil, invalid("end", "work_on, start and end are required")
	}
	var m model.AttendanceEntry
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		c, err := s.meContractorOf(ctx, tx)
		if err != nil {
			return err
		}
		uid := auth.UserFrom(ctx).ID
		m = model.AttendanceEntry{AccountID: c.AccountID, ContractorID: c.ID, Status: model.AttendanceDraft,
			Source: model.SourceManual, RateSnapshot: c.HourlyRate, KmRateSnapshot: c.KmRate, CreatedBy: &uid}
		if err := applyAttendanceInput(&m, b); err != nil {
			return err
		}
		if _, err := s.checkJobForContractor(ctx, tx, m.JobID, c.ID, false); err != nil {
			return err
		}
		if err := s.validateEntry(ctx, tx, &m); err != nil {
			return err
		}
		return dbErrOrNil(tx.Create(&m).Error, "attendance entry")
	})
	if err != nil {
		return nil, err
	}
	return &Out[AttendanceEntry]{Body: s.attendanceOut(ctx, &m, false)}, nil
}

func (s *server) mePatchAttendance(ctx context.Context, in *struct {
	ID   uint `path:"id"`
	Body AttendanceInput
}) (*Out[AttendanceEntry], error) {
	var m *model.AttendanceEntry
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		c, err := s.meContractorOf(ctx, tx)
		if err != nil {
			return err
		}
		if m, err = s.loadEntry(ctx, tx, in.ID); err != nil || m.ContractorID != c.ID {
			return notFound("attendance entry")
		}
		if !attendance.Editable(toDomain(m), false, auth.AccountFrom(ctx).AttendanceLockedThrough) {
			return conflict(CodeNotEditable, "the entry was submitted or approved; ask your manager")
		}
		b := in.Body
		b.ContractorID, b.Status, b.RateSnapshot, b.KmRateSnapshot = nil, nil, nil, nil // staff-only fields
		if err := applyAttendanceInput(m, b); err != nil {
			return err
		}
		if m.Status == model.AttendanceRejected { // fixing a rejected entry makes it a draft again
			m.Status, m.RejectReason = model.AttendanceDraft, ""
		}
		if _, err := s.checkJobForContractor(ctx, tx, m.JobID, c.ID, false); err != nil {
			return err
		}
		if err := s.validateEntry(ctx, tx, m); err != nil {
			return err
		}
		return dbErrOrNil(tx.Save(m).Error, "attendance entry")
	})
	if err != nil {
		return nil, err
	}
	return &Out[AttendanceEntry]{Body: s.attendanceOut(ctx, m, false)}, nil
}

func (s *server) meDeleteAttendance(ctx context.Context, in *idPath) (*NoContent, error) {
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		c, err := s.meContractorOf(ctx, tx)
		if err != nil {
			return err
		}
		m, err := s.loadEntry(ctx, tx, in.ID)
		if err != nil || m.ContractorID != c.ID {
			return notFound("attendance entry")
		}
		if !attendance.Editable(toDomain(m), false, auth.AccountFrom(ctx).AttendanceLockedThrough) {
			return conflict(CodeNotEditable, "the entry was submitted or approved; ask your manager")
		}
		return dbErrOrNil(tx.Delete(m).Error, "attendance entry")
	})
	if err != nil {
		return nil, err
	}
	return &NoContent{}, nil
}

func (s *server) clockIn(ctx context.Context, in *struct{ Body ClockIn }) (*Out[AttendanceEntry], error) {
	var m model.AttendanceEntry
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		c, err := s.meContractorOf(ctx, tx)
		if err != nil {
			return err
		}
		open, err := s.openEntry(ctx, tx, c.ID)
		if err != nil {
			return err
		}
		if open != nil {
			return conflict(CodeAlreadyClockedIn, "you are already clocked in since "+attendance.FormatClock(open.StartMin))
		}
		today, minute := s.meNow(ctx)
		uid := auth.UserFrom(ctx).ID
		m = model.AttendanceEntry{AccountID: c.AccountID, ContractorID: c.ID, JobID: in.Body.JobID, WorkOn: today,
			StartMin: minute, Status: model.AttendanceDraft, Source: model.SourceClock,
			RateSnapshot: c.HourlyRate, KmRateSnapshot: c.KmRate, CreatedBy: &uid}
		if auth.AccountFrom(ctx).AttendanceGPS {
			m.LatIn, m.LngIn = in.Body.Lat, in.Body.Lng
		}
		job, err := s.checkJobForContractor(ctx, tx, m.JobID, c.ID, false)
		if err != nil {
			return err
		}
		if err := s.validateEntry(ctx, tx, &m); err != nil {
			return err
		}
		if err := tx.Create(&m).Error; err != nil {
			return dbErr(err, "attendance entry")
		}
		return startJobOnWork(ctx, tx, job)
	})
	if err != nil {
		return nil, err
	}
	return &Out[AttendanceEntry]{Body: s.attendanceOut(ctx, &m, false)}, nil
}

func (s *server) clockOut(ctx context.Context, in *struct{ Body ClockOut }) (*Out[AttendanceEntry], error) {
	var m *model.AttendanceEntry
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		c, err := s.meContractorOf(ctx, tx)
		if err != nil {
			return err
		}
		if m, err = s.openEntry(ctx, tx, c.ID); err != nil {
			return err
		}
		if m == nil {
			return conflict(CodeNotClockedIn, "you are not clocked in")
		}
		today, minute := s.meNow(ctx)
		if m.WorkOn != today {
			return conflict(CodeOpenEntryPastDay, "the open entry is from "+czDate(m.WorkOn)+"; fill in its end time instead")
		}
		end := max(minute, m.StartMin+1)
		m.EndMin = &end
		m.BreakMin, m.KM = in.Body.BreakMin, in.Body.KM
		if note := strings.TrimSpace(in.Body.Note); note != "" {
			m.Note = note
		}
		if auth.AccountFrom(ctx).AttendanceGPS {
			m.LatOut, m.LngOut = in.Body.Lat, in.Body.Lng
		}
		if err := s.validateEntry(ctx, tx, m); err != nil {
			return err
		}
		return dbErrOrNil(tx.Save(m).Error, "attendance entry")
	})
	if err != nil {
		return nil, err
	}
	return &Out[AttendanceEntry]{Body: s.attendanceOut(ctx, m, false)}, nil
}

func (s *server) meSubmit(ctx context.Context, in *struct{ Body AttendanceIDs }) (*Out[ListResponse[AttendanceEntry]], error) {
	c, err := s.meContractorOf(ctx, s.db.WithContext(ctx))
	if err != nil {
		return nil, err
	}
	return s.transitionMany(ctx, in.Body.IDs, model.AttendanceSubmitted, "", events.AttendanceSubmitted, "odeslána ke schválení", false, &c.ID)
}
