package api_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/qwerin/zahrada/internal/api"
)

func TestContractorAdvances(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	b := ts.signup("b@example.cz", "Firma B")
	cl := newClient(a, "Klient")
	petr := newContractor(a, "Petr", 30000)
	job := newJob(a, api.JobCreate{ClientID: cl.ID, Kind: "realization", Title: "Trávník", Status: "ordered", ContractorIDs: []uint{petr.ID}})
	// březen: 2× 8 h schváleno = 2 × 2 400 Kč; duben: 4 h = 1 200 Kč
	for _, on := range []string{"2026-03-02", "2026-03-03"} {
		doJSON[api.AttendanceEntry](a, http.StatusCreated, "POST", a.acct("/attendance"), entry(petr.ID, &job.ID, on, "07:00", "15:00", 0))
	}
	doJSON[api.AttendanceEntry](a, http.StatusCreated, "POST", a.acct("/attendance"), entry(petr.ID, nil, "2026-04-01", "08:00", "12:00", 0))

	pay := func(kind, on string, amount int64) api.ContractorPayment {
		return doJSON[api.ContractorPayment](a, http.StatusCreated, "POST", path(a, "/contractors/%d/payments", petr.ID),
			api.ContractorPaymentInput{Kind: kind, PaidOn: on, Amount: amount, Method: "cash", Note: "na nářadí"})
	}
	adv := pay("advance", "2026-03-05", 200000)
	res, body := a.do("POST", path(a, "/contractors/%d/payments", petr.ID), api.ContractorPaymentInput{Kind: "advance", PaidOn: "2026-03-05", Amount: 0})
	assertError(t, res, body, http.StatusUnprocessableEntity, "amount")

	sum := doJSON[api.AttendanceSummary](a, http.StatusOK, "GET", a.acct("/attendance/summary?month=2026-03"), nil)
	c := sum.Contractors[0]
	if c.Total != 480000 || c.Advances != 200000 || c.ToPay != 280000 || c.Balance != 280000 || sum.TotalToPay != 280000 {
		t.Fatalf("march: %+v", c)
	}
	pay("payout", "2026-03-31", 280000)
	got := doJSON[api.ContractorPayments](a, http.StatusOK, "GET", path(a, "/contractors/%d/payments", petr.ID), nil)
	// celkem odpracováno 6 000 Kč (březen + duben), vyplaceno 4 800 Kč
	if len(got.Items) != 2 || got.Balance.Earned != 600000 || got.Balance.Paid != 480000 || got.Balance.Balance != 120000 {
		t.Fatalf("balance: %+v", got)
	}
	// záloha v měsíci bez odpracovaných hodin má svůj řádek
	pay("advance", "2026-05-02", 50000)
	may := doJSON[api.AttendanceSummary](a, http.StatusOK, "GET", a.acct("/attendance/summary?month=2026-05"), nil)
	if len(may.Contractors) != 1 || may.Contractors[0].ToPay != -50000 || may.Contractors[0].Balance != 70000 {
		t.Fatalf("may: %+v", may.Contractors)
	}

	// uzavřené období nelze měnit
	a.mustDo(http.StatusOK, "POST", a.acct("/attendance/actions/close"), api.AttendanceClose{Through: "2026-03-31"})
	res, body = a.do("DELETE", path(a, "/contractor-payments/%d", adv.ID), nil)
	assertCode(t, res, body, http.StatusConflict, "period_closed")
	res, body = a.do("POST", path(a, "/contractors/%d/payments", petr.ID), api.ContractorPaymentInput{Kind: "advance", PaidOn: "2026-03-20", Amount: 100})
	assertCode(t, res, body, http.StatusConflict, "period_closed")

	// dodavatel vidí svoje zálohy a zůstatek, ne cizí firmy
	d := contractorLogin(t, ts, a, petr.ID, "petr@example.cz")
	mine := doJSON[api.ContractorPayments](d, http.StatusOK, "GET", d.acct("/me/payments"), nil)
	if len(mine.Items) != 3 || mine.Balance.Balance != 70000 || mine.Items[0].Editable {
		t.Fatalf("me: %+v", mine)
	}
	d.mustDo(http.StatusForbidden, "GET", path(d, "/contractors/%d/payments", petr.ID), nil)
	b.mustDo(http.StatusNotFound, "GET", path(b, "/contractors/%d/payments", petr.ID), nil)
	b.mustDo(http.StatusNotFound, "POST", path(b, "/contractors/%d/payments", petr.ID), api.ContractorPaymentInput{Kind: "advance", PaidOn: "2026-04-02", Amount: 100})
	res, body = a.do("DELETE", path(a, "/contractors/%d", petr.ID), nil)
	assertCode(t, res, body, http.StatusConflict, "has_attendance")
}

func TestJobDocuments(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	cl := newClient(a, "Klient")
	job := newJob(a, api.JobCreate{ClientID: cl.ID, Kind: "realization", Title: "Trávník"})
	docx := append([]byte("PK\x03\x04"), make([]byte, 100)...)
	for name, data := range map[string][]byte{
		"smlouva.pdf": []byte("%PDF-1.4\n%…"), "plan.docx": docx, "rozpocet.xlsx": docx, "poznamky.txt": []byte("Pozor na psa\n"),
	} {
		uploadOK(a, "job", job.ID, name, data)
	}
	// office jen u zakázek a podle obsahu: přejmenovaný binární soubor neprojde
	res, body := a.upload(map[string]string{"owner_type": "client", "owner_id": itoa(cl.ID)}, "plan.docx", docx)
	assertError(t, res, body, http.StatusUnsupportedMediaType, "")
	res, body = a.upload(map[string]string{"owner_type": "job", "owner_id": itoa(job.ID)}, "virus.docx", []byte("MZ\x90\x00binary"))
	assertError(t, res, body, http.StatusUnsupportedMediaType, "")
	list := doJSON[api.ListResponse[api.Attachment]](a, http.StatusOK, "GET", a.acct("/attachments?owner_type=job&owner_id="+itoa(job.ID)), nil)
	types := []string{}
	for _, it := range list.Items {
		types = append(types, it.ContentType)
	}
	if list.Total != 4 || !strings.Contains(strings.Join(types, ","), "wordprocessingml") {
		t.Fatalf("documents: %v", types)
	}
}
