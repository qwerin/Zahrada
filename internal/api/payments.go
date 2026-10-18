package api

// Payments to contractors — advances (zálohy) and payouts (doplatky). The
// balance of a contractor is what the firm still owes: approved attendance
// earnings (labour + km, rounded per entry) minus everything paid.

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"gorm.io/gorm"

	"github.com/qwerin/zahrada/internal/attendance"
	"github.com/qwerin/zahrada/internal/auth"
	"github.com/qwerin/zahrada/internal/events"
	"github.com/qwerin/zahrada/internal/model"
)

type ContractorPayment struct {
	ID             uint      `json:"id"`
	ContractorID   uint      `json:"contractor_id"`
	ContractorName string    `json:"contractor_name,omitempty"`
	Kind           string    `json:"kind" enum:"advance,payout" doc:"advance = záloha, payout = výplata/doplatek"`
	PaidOn         string    `json:"paid_on" format:"date"`
	Amount         int64     `json:"amount" doc:"Haléře"`
	Method         string    `json:"method" enum:"cash,bank,"`
	Note           string    `json:"note"`
	Editable       bool      `json:"editable" doc:"Not in a closed period"`
	CreatedAt      time.Time `json:"created_at"`
}

type ContractorPaymentInput struct {
	Kind   string `json:"kind" enum:"advance,payout"`
	PaidOn string `json:"paid_on" format:"date"`
	Amount int64  `json:"amount" minimum:"1" doc:"Haléře"`
	Method string `json:"method,omitempty" enum:"cash,bank,"`
	Note   string `json:"note,omitempty" maxLength:"500"`
}

// ContractorBalance: Earned (approved attendance) − Paid = Balance (positive = the firm owes the contractor).
type ContractorBalance struct {
	Through string `json:"through,omitempty" doc:"Balance as of this day (empty = all)"`
	Earned  int64  `json:"earned"`
	Paid    int64  `json:"paid"`
	Balance int64  `json:"balance" doc:"Positive = still to pay, negative = paid in advance"`
	Pending int64  `json:"pending_count" doc:"Attendance entries not approved yet (not in Earned)"`
}

type ContractorPayments struct {
	Items   []ContractorPayment `json:"items" nullable:"false"`
	Balance ContractorBalance   `json:"balance"`
}

func (s *server) registerPayments(g huma.API) {
	huma.Get(g, "/contractors/{id}/payments", s.listContractorPayments)
	huma.Post(g, "/contractors/{id}/payments", s.createContractorPayment, status(http.StatusCreated), auth.ForEditors)
	huma.Delete(g, "/contractor-payments/{id}", s.deleteContractorPayment, status(http.StatusNoContent), auth.ForEditors)
	huma.Get(g, "/me/payments", s.mePayments, auth.ForEveryone)
}

func (s *server) toPayment(ctx context.Context, m *model.ContractorPayment) ContractorPayment {
	return ContractorPayment{ID: m.ID, ContractorID: m.ContractorID, Kind: m.Kind, PaidOn: m.PaidOn, Amount: m.Amount,
		Method: m.Method, Note: m.Note, CreatedAt: m.CreatedAt,
		Editable: !attendance.Locked(m.PaidOn, auth.AccountFrom(ctx).AttendanceLockedThrough)}
}

// contractorBalance computes the balance of one contractor up to through ("" = everything).
func (s *server) contractorBalance(ctx context.Context, db *gorm.DB, contractorID uint, through string) (ContractorBalance, error) {
	out := ContractorBalance{Through: through}
	q := db.Scopes(inAccount(ctx)).Where("contractor_id = ?", contractorID)
	if through != "" {
		q = q.Where("work_on <= ?", through)
	}
	var rows []model.AttendanceEntry
	if err := q.Session(&gorm.Session{}).Where("status = ? AND end_min IS NOT NULL", model.AttendanceApproved).Find(&rows).Error; err != nil {
		return out, dbErr(err, "attendance")
	}
	if err := q.Session(&gorm.Session{}).Model(&model.AttendanceEntry{}).Where("status <> ? OR end_min IS NULL", model.AttendanceApproved).
		Count(&out.Pending).Error; err != nil {
		return out, dbErr(err, "attendance")
	}
	entries := make([]attendance.Entry, len(rows))
	for i := range rows {
		entries[i] = toDomain(&rows[i])
	}
	for _, c := range attendance.Summarize(entries, s.rounding(ctx)) {
		out.Earned += c.Total
	}
	pq := db.Model(&model.ContractorPayment{}).Scopes(inAccount(ctx)).Where("contractor_id = ?", contractorID)
	if through != "" {
		pq = pq.Where("paid_on <= ?", through)
	}
	if err := pq.Select("COALESCE(SUM(amount), 0)").Scan(&out.Paid).Error; err != nil {
		return out, dbErr(err, "payment")
	}
	out.Balance = out.Earned - out.Paid
	return out, nil
}

func (s *server) paymentsOf(ctx context.Context, db *gorm.DB, contractorID uint) (ContractorPayments, error) {
	var rows []model.ContractorPayment
	if err := db.Scopes(inAccount(ctx)).Where("contractor_id = ?", contractorID).Order("paid_on DESC, id DESC").Find(&rows).Error; err != nil {
		return ContractorPayments{}, dbErr(err, "payment")
	}
	out := ContractorPayments{Items: make([]ContractorPayment, len(rows))}
	for i := range rows {
		out.Items[i] = s.toPayment(ctx, &rows[i])
	}
	bal, err := s.contractorBalance(ctx, db, contractorID, "")
	out.Balance = bal
	return out, err
}

func (s *server) listContractorPayments(ctx context.Context, in *idPath) (*Out[ContractorPayments], error) {
	db := s.db.WithContext(ctx)
	if _, err := s.loadContractor(ctx, db, in.ID); err != nil {
		return nil, err
	}
	out, err := s.paymentsOf(ctx, db, in.ID)
	return &Out[ContractorPayments]{Body: out}, err
}

func (s *server) createContractorPayment(ctx context.Context, in *struct {
	ID   uint `path:"id"`
	Body ContractorPaymentInput
}) (*Out[ContractorPayment], error) {
	b := in.Body
	if !validDate(b.PaidOn) {
		return nil, invalid("paid_on", "use YYYY-MM-DD")
	}
	acc := auth.AccountFrom(ctx)
	if attendance.Locked(b.PaidOn, acc.AttendanceLockedThrough) {
		return nil, conflict(CodePeriodClosed, "the period is closed through "+acc.AttendanceLockedThrough)
	}
	var m model.ContractorPayment
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		c, err := s.loadContractor(ctx, tx, in.ID)
		if err != nil {
			return err
		}
		uid := auth.UserFrom(ctx).ID
		m = model.ContractorPayment{AccountID: acc.ID, ContractorID: c.ID, Kind: b.Kind, PaidOn: b.PaidOn, Amount: b.Amount,
			Method: b.Method, Note: strings.TrimSpace(b.Note), CreatedBy: &uid}
		if err := tx.Create(&m).Error; err != nil {
			return dbErr(err, "payment")
		}
		what := map[string]string{model.PaymentAdvance: "záloha", model.PaymentPayout: "výplata"}[b.Kind]
		return recordSimple(ctx, tx, events.ContractorPaid, events.SubjectContractor, c.ID,
			fmt.Sprintf("Dodavateli %s vyplacena %s %s Kč (%s)", c.Name, what, czDecimal(b.Amount, 2), czDate(b.PaidOn)),
			map[string]any{"payment_id": m.ID, "kind": b.Kind, "amount": b.Amount})
	})
	if err != nil {
		return nil, err
	}
	return &Out[ContractorPayment]{Body: s.toPayment(ctx, &m)}, nil
}

func (s *server) deleteContractorPayment(ctx context.Context, in *idPath) (*NoContent, error) {
	var m model.ContractorPayment
	if err := s.scoped(ctx).First(&m, in.ID).Error; err != nil {
		return nil, dbErr(err, "payment")
	}
	if attendance.Locked(m.PaidOn, auth.AccountFrom(ctx).AttendanceLockedThrough) {
		return nil, conflict(CodePeriodClosed, "the period is closed")
	}
	if err := s.db.WithContext(ctx).Delete(&m).Error; err != nil {
		return nil, dbErr(err, "payment")
	}
	return &NoContent{}, nil
}

func (s *server) mePayments(ctx context.Context, _ *struct{}) (*Out[ContractorPayments], error) {
	db := s.db.WithContext(ctx)
	c, err := s.meContractorOf(ctx, db)
	if err != nil {
		return nil, err
	}
	out, err := s.paymentsOf(ctx, db, c.ID)
	for i := range out.Items {
		out.Items[i].Editable = false
	}
	return &Out[ContractorPayments]{Body: out}, err
}

// monthPayments sums payments per contractor and kind in [from, to].
func (s *server) monthPayments(ctx context.Context, from, to string, contractorID uint) (map[uint]map[string]int64, error) {
	var rows []struct {
		ContractorID uint
		Kind         string
		Sum          int64
	}
	q := s.scoped(ctx).Model(&model.ContractorPayment{}).Where("paid_on BETWEEN ? AND ?", from, to)
	if contractorID != 0 {
		q = q.Where("contractor_id = ?", contractorID)
	}
	if err := q.Select("contractor_id, kind, SUM(amount) AS sum").Group("contractor_id, kind").Scan(&rows).Error; err != nil {
		return nil, dbErr(err, "payment")
	}
	out := map[uint]map[string]int64{}
	for _, r := range rows {
		if out[r.ContractorID] == nil {
			out[r.ContractorID] = map[string]int64{}
		}
		out[r.ContractorID][r.Kind] = r.Sum
	}
	return out, nil
}
