package api_test

import (
	"bytes"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/qwerin/zahrada/internal/api"
	"github.com/qwerin/zahrada/internal/auth"
	"github.com/qwerin/zahrada/internal/config"
	"github.com/qwerin/zahrada/internal/model"
)

func TestClientsAndSites(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	b := ts.signup("b@example.cz", "Firma B")

	c := newClient(a, "Jana Nováková")
	site := newSite(a, c.ID, "Zahrada Říčany")
	if site.ClientName != "Jana Nováková" || site.AccessNote != "klíč pod rohožkou" {
		t.Fatalf("site: %+v", site)
	}
	d := doJSON[api.ClientDetail](a, http.StatusOK, "GET", path(a, "/clients/%d", c.ID), nil)
	if len(d.Sites) != 1 || d.Sites[0].ID != site.ID {
		t.Fatalf("detail: %+v", d)
	}
	d = doJSON[api.ClientDetail](a, http.StatusOK, "PATCH", path(a, "/clients/%d", c.ID), map[string]any{"phone": " 777 123 456 "})
	if d.Phone != "777 123 456" || d.Name != "Jana Nováková" {
		t.Fatalf("patched: %+v", d)
	}
	// diacritics-insensitive search
	list := doJSON[api.ListResponse[api.Client]](a, http.StatusOK, "GET", a.acct("/clients?query=novakova"), nil)
	if list.Total != 1 {
		t.Fatalf("search: %+v", list)
	}
	sites := doJSON[api.ListResponse[api.Site]](a, http.StatusOK, "GET", a.acct("/sites?query=ricany"), nil)
	if sites.Total != 1 || sites.Items[0].ClientName != "Jana Nováková" {
		t.Fatalf("site search: %+v", sites)
	}

	// another account sees nothing and cannot attach a garden to a foreign client
	b.mustDo(http.StatusNotFound, "GET", path(b, "/clients/%d", c.ID), nil)
	b.mustDo(http.StatusNotFound, "GET", path(b, "/sites/%d", site.ID), nil)
	b.mustDo(http.StatusNotFound, "PATCH", path(b, "/sites/%d", site.ID), map[string]any{"name": "x"})
	res, body := b.do("POST", b.acct("/sites"), api.SiteCreate{ClientID: c.ID, Name: "Cizí"})
	assertError(t, res, body, http.StatusUnprocessableEntity, "client_id")
	if l := doJSON[api.ListResponse[api.Client]](b, http.StatusOK, "GET", b.acct("/clients"), nil); l.Total != 0 {
		t.Fatalf("B sees clients: %+v", l)
	}

	// a client with jobs cannot be deleted; without them it goes with its gardens
	job := newJob(a, api.JobCreate{ClientID: c.ID, SiteID: &site.ID, Kind: "maintenance", Title: "Sečení"})
	res, body = a.do("DELETE", path(a, "/clients/%d", c.ID), nil)
	assertCode(t, res, body, http.StatusConflict, "has_jobs")
	res, body = a.do("DELETE", path(a, "/sites/%d", site.ID), nil)
	assertCode(t, res, body, http.StatusConflict, "has_jobs")
	a.mustDo(http.StatusNoContent, "DELETE", path(a, "/jobs/%d", job.ID), nil)
	a.mustDo(http.StatusNoContent, "DELETE", path(a, "/clients/%d", c.ID), nil)
	a.mustDo(http.StatusNotFound, "GET", path(a, "/sites/%d", site.ID), nil)
}

func TestJobs(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	c := newClient(a, "Klient")
	other := newClient(a, "Jiný klient")
	site := newSite(a, c.ID, "Zahrada")
	petr := newContractor(a, "Petr", 35000)

	j1 := newJob(a, api.JobCreate{ClientID: c.ID, SiteID: &site.ID, Kind: "realization", Title: "Nový trávník",
		PlannedFrom: "2026-04-01", PlannedTo: "2026-04-10", ContractorIDs: []uint{petr.ID, petr.ID}})
	j2 := newJob(a, api.JobCreate{ClientID: c.ID, Kind: "one_off", Title: "Prořez"})
	if j1.Number != "Z2026-001" || j2.Number != "Z2026-002" || j1.Status != "inquiry" || j1.ClientName != "Klient" ||
		j1.SiteName != "Zahrada" || !slices.Equal(j1.ContractorIDs, []uint{petr.ID}) {
		t.Fatalf("jobs: %+v %+v", j1, j2)
	}
	if !slices.Contains(j1.NextStatuses, "ordered") || slices.Contains(j1.NextStatuses, "invoiced") {
		t.Fatalf("next statuses: %v", j1.NextStatuses)
	}

	// validation: garden of another client, inverted dates, unknown contractor
	res, body := a.do("POST", a.acct("/jobs"), api.JobCreate{ClientID: other.ID, SiteID: &site.ID, Kind: "one_off", Title: "x"})
	assertError(t, res, body, http.StatusUnprocessableEntity, "site_id")
	res, body = a.do("POST", a.acct("/jobs"), api.JobCreate{ClientID: c.ID, Kind: "one_off", Title: "x", PlannedFrom: "2026-05-02", PlannedTo: "2026-05-01"})
	assertError(t, res, body, http.StatusUnprocessableEntity, "planned_to")
	res, body = a.do("PATCH", path(a, "/jobs/%d", j1.ID), map[string]any{"contractor_ids": []uint{9999}})
	assertError(t, res, body, http.StatusUnprocessableEntity, "contractor_ids")

	// status graph
	j := doJSON[api.Job](a, http.StatusOK, "POST", path(a, "/jobs/%d/status", j1.ID), api.JobStatusChange{Status: "ordered"})
	if j.Status != "ordered" {
		t.Fatalf("ordered: %+v", j)
	}
	res, body = a.do("POST", path(a, "/jobs/%d/status", j1.ID), api.JobStatusChange{Status: "invoiced"})
	assertCode(t, res, body, http.StatusConflict, "invalid_transition")

	open := doJSON[api.ListResponse[api.Job]](a, http.StatusOK, "GET", a.acct("/jobs?status=open&contractor_id="+itoa(petr.ID)), nil)
	if open.Total != 1 || open.Items[0].ID != j1.ID {
		t.Fatalf("filter: %+v", open)
	}
	j = doJSON[api.Job](a, http.StatusOK, "PATCH", path(a, "/jobs/%d", j1.ID), map[string]any{"site_id": 0, "contractor_ids": []uint{}})
	if j.SiteID != nil || len(j.ContractorIDs) != 0 || j.Status != "ordered" {
		t.Fatalf("patched: %+v", j)
	}
	ev := doJSON[api.ListResponse[api.Event]](a, http.StatusOK, "GET", a.acct("/events?subject_type=job&subject_id="+itoa(j1.ID)), nil)
	if ev.Total < 3 {
		t.Fatalf("events: %+v", ev)
	}
}

func itoa(v uint) string { return strconv.FormatUint(uint64(v), 10) }

func entry(contractor uint, job *uint, on, from, to string, brk int) api.AttendanceInput {
	return api.AttendanceInput{ContractorID: &contractor, JobID: job, WorkOn: &on, Start: &from, End: &to, BreakMin: &brk}
}

func TestAttendanceManager(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	c := newClient(a, "Klient")
	petr := newContractor(a, "Petr", 35000)
	job := newJob(a, api.JobCreate{ClientID: c.ID, Kind: "realization", Title: "Trávník", Status: "ordered"})

	e1 := doJSON[api.AttendanceEntry](a, http.StatusCreated, "POST", a.acct("/attendance"), entry(petr.ID, &job.ID, "2026-03-02", "7:00", "15:07", 30))
	// 457 min → rounded to 450 (15 min, nearest) → 7.5 h × 350 Kč
	if e1.Status != "approved" || e1.WorkedMin != 450 || e1.Cost != 262500 || e1.ContractorName != "Petr" || e1.JobNumber != job.Number {
		t.Fatalf("entry: %+v", e1)
	}
	if j := doJSON[api.Job](a, http.StatusOK, "GET", path(a, "/jobs/%d", job.ID), nil); j.Status != "in_progress" {
		t.Fatalf("first attendance should start the job: %s", j.Status)
	}
	res, body := a.do("POST", a.acct("/attendance"), entry(petr.ID, nil, "2026-03-02", "14:00", "16:00", 0))
	assertCode(t, res, body, http.StatusConflict, "attendance_overlap")
	res, body = a.do("POST", a.acct("/attendance"), entry(petr.ID, nil, "2026-03-02", "16:00", "15:00", 0))
	assertError(t, res, body, http.StatusUnprocessableEntity, "end")
	res, body = a.do("POST", a.acct("/attendance"), entry(petr.ID, nil, "2026-03-02", "15:07", "16:00", 60))
	assertError(t, res, body, http.StatusUnprocessableEntity, "break_min")

	draft := "draft"
	in := entry(petr.ID, nil, "2026-03-03", "08:00", "12:00", 0)
	in.Status = &draft
	e2 := doJSON[api.AttendanceEntry](a, http.StatusCreated, "POST", a.acct("/attendance"), in)

	sum := doJSON[api.AttendanceSummary](a, http.StatusOK, "GET", a.acct("/attendance/summary?month=2026-03"), nil)
	if sum.PendingCount != 1 || len(sum.Contractors) != 1 || sum.Contractors[0].Minutes != 450 || sum.TotalCost != 262500 {
		t.Fatalf("summary: %+v", sum)
	}

	// closing with a pending entry fails; approve, then close
	res, body = a.do("POST", a.acct("/attendance/actions/close"), api.AttendanceClose{Through: "2026-03-31"})
	assertCode(t, res, body, http.StatusConflict, "pending_entries")
	// a manager may approve a draft directly; approving it again is not a transition
	doJSON[api.ListResponse[api.AttendanceEntry]](a, http.StatusOK, "POST", a.acct("/attendance/actions/approve"), api.AttendanceIDs{IDs: []uint{e2.ID}})
	res, body = a.do("POST", a.acct("/attendance/actions/approve"), api.AttendanceIDs{IDs: []uint{e2.ID}})
	assertCode(t, res, body, http.StatusConflict, "invalid_transition")
	acc := doJSON[api.Account](a, http.StatusOK, "POST", a.acct("/attendance/actions/close"), api.AttendanceClose{Through: "2026-03-31"})
	if acc.AttendanceLockedThrough != "2026-03-31" {
		t.Fatalf("closed: %+v", acc)
	}
	res, body = a.do("PATCH", path(a, "/attendance/%d", e1.ID), map[string]any{"note": "x"})
	assertCode(t, res, body, http.StatusConflict, "period_closed")
	res, body = a.do("POST", a.acct("/attendance"), entry(petr.ID, nil, "2026-03-20", "08:00", "09:00", 0))
	assertCode(t, res, body, http.StatusConflict, "period_closed")
	sum = doJSON[api.AttendanceSummary](a, http.StatusOK, "GET", a.acct("/attendance/summary?month=2026-03"), nil)
	if !sum.Locked || sum.PendingCount != 0 || sum.Contractors[0].Minutes != 690 || len(sum.Contractors[0].Jobs) != 2 {
		t.Fatalf("summary after close: %+v", sum)
	}

	res, body = a.do("GET", a.acct("/attendance/export.csv?month=2026-03"), nil)
	if res.StatusCode != http.StatusOK || !bytes.HasPrefix(body, []byte("\ufeffDodavatel;Datum")) ||
		!strings.Contains(string(body), "2. 3. 2026;07:00;15:07;30;7,50;") || !strings.Contains(res.Header.Get("Content-Disposition"), "dochazka-2026-03.csv") {
		t.Fatalf("csv: %d %s", res.StatusCode, body)
	}
	// a job or contractor with attendance cannot be deleted
	res, body = a.do("DELETE", path(a, "/jobs/%d", job.ID), nil)
	assertCode(t, res, body, http.StatusConflict, "has_attendance")
	res, body = a.do("DELETE", path(a, "/contractors/%d", petr.ID), nil)
	assertCode(t, res, body, http.StatusConflict, "has_attendance")
}

// contractorLogin invites a contractor, accepts as a new user and returns their client.
func contractorLogin(t *testing.T, ts *testServer, owner *client, contractorID uint, email string) *client {
	t.Helper()
	doJSON[api.Invitation](owner, http.StatusCreated, "POST", path(owner, "/contractors/%d/invite", contractorID), api.ContractorInvite{Email: email})
	msg, _ := ts.mail.Last()
	m := inviteLink.FindStringSubmatch(msg.Text)
	if m == nil || !strings.Contains(msg.Text, "role: dodavatel") {
		t.Fatalf("contractor invitation e-mail: %+v", msg)
	}
	dc := ts.signup(email, "Vlastní "+email)
	verify(ts, email)
	dc.mustDo(http.StatusOK, "POST", "/api/invitations/"+m[1]+"/accept", nil)
	dc.slug = owner.slug
	return dc
}

func TestContractorSelfService(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	cl := newClient(a, "Klient")
	site := newSite(a, cl.ID, "Zahrada Říčany")
	petr := newContractor(a, "Petr", 36000)
	pavel := newContractor(a, "Pavel", 30000)
	mine := newJob(a, api.JobCreate{ClientID: cl.ID, SiteID: &site.ID, Kind: "realization", Title: "Trávník", Status: "ordered", ContractorIDs: []uint{petr.ID}})
	notMine := newJob(a, api.JobCreate{ClientID: cl.ID, Kind: "one_off", Title: "Cizí", Status: "ordered", ContractorIDs: []uint{pavel.ID}})
	doJSON[api.AttendanceEntry](a, http.StatusCreated, "POST", a.acct("/attendance"), entry(pavel.ID, nil, "2026-03-02", "07:00", "08:00", 0))

	d := contractorLogin(t, ts, a, petr.ID, "petr@example.cz")
	if got := doJSON[api.Contractor](a, http.StatusOK, "GET", path(a, "/contractors/%d", petr.ID), nil); got.UserEmail != "petr@example.cz" {
		t.Fatalf("not linked: %+v", got)
	}
	acc := doJSON[api.Account](d, http.StatusOK, "GET", d.acct(""), nil)
	if acc.Role != "contractor" || acc.Capabilities.Staff || acc.Capabilities.Edit {
		t.Fatalf("contractor account: %+v", acc)
	}
	// nothing outside /me
	for _, p := range []string{"/clients", "/jobs", "/attendance", "/contractors", "/members", "/events", "/search?q=x", "/attachments"} {
		res, body := d.do("GET", d.acct(p), nil)
		assertError(t, res, body, http.StatusForbidden, "")
	}
	res, body := d.do("POST", d.acct("/calc/preview"), api.CalcPreviewInput{Items: []api.CalcItem{}})
	assertError(t, res, body, http.StatusForbidden, "")

	jobs := doJSON[[]api.MeJob](d, http.StatusOK, "GET", d.acct("/me/jobs"), nil)
	if len(jobs) != 1 || jobs[0].ID != mine.ID || jobs[0].AccessNote != "klíč pod rohožkou" || jobs[0].Address == "" {
		t.Fatalf("my jobs: %+v", jobs)
	}

	// clock in at 11:00 Prague (10:00 UTC, winter time)
	res, body = d.do("POST", d.acct("/me/attendance/clock-in"), api.ClockIn{JobID: &notMine.ID})
	assertCode(t, res, body, http.StatusConflict, "job_not_assigned")
	open := doJSON[api.AttendanceEntry](d, http.StatusCreated, "POST", d.acct("/me/attendance/clock-in"), api.ClockIn{JobID: &mine.ID})
	if open.Start != "11:00" || open.End != nil || open.WorkOn != "2026-03-15" || open.Status != "draft" || open.Source != "clock" {
		t.Fatalf("clock-in: %+v", open)
	}
	res, body = d.do("POST", d.acct("/me/attendance/clock-in"), api.ClockIn{})
	assertCode(t, res, body, http.StatusConflict, "already_clocked_in")
	if j := doJSON[api.Job](a, http.StatusOK, "GET", path(a, "/jobs/%d", mine.ID), nil); j.Status != "in_progress" {
		t.Fatalf("job not started: %s", j.Status)
	}
	me := doJSON[api.MeContractor](d, http.StatusOK, "GET", d.acct("/me/contractor"), nil)
	if me.Open == nil || me.Open.ID != open.ID || me.Name != "Petr" {
		t.Fatalf("me: %+v", me)
	}

	ts.now = ts.now.Add(5*time.Hour + 7*time.Minute) // 16:07
	closed := doJSON[api.AttendanceEntry](d, http.StatusOK, "POST", d.acct("/me/attendance/clock-out"), api.ClockOut{BreakMin: 30, KM: 12})
	if closed.End == nil || *closed.End != "16:07" || closed.WorkedMin != 270 || closed.Cost != 162000 || !closed.Editable {
		t.Fatalf("clock-out: %+v", closed)
	}
	res, body = d.do("POST", d.acct("/me/attendance/clock-out"), api.ClockOut{})
	assertCode(t, res, body, http.StatusConflict, "not_clocked_in")

	// own manual entry; cannot touch Pavel's entries or set staff-only fields
	on, from, to := "2026-03-14", "08:00", "10:00"
	own := doJSON[api.AttendanceEntry](d, http.StatusCreated, "POST", d.acct("/me/attendance"),
		api.AttendanceInput{WorkOn: &on, Start: &from, End: &to, Status: ptr("approved"), RateSnapshot: ptr(int64(99999))})
	if own.Status != "draft" || own.RateSnapshot != 36000 || own.ContractorID != petr.ID {
		t.Fatalf("own entry: %+v", own)
	}
	list := doJSON[api.ListResponse[api.AttendanceEntry]](d, http.StatusOK, "GET", d.acct("/me/attendance?month=2026-03"), nil)
	if list.Total != 2 {
		t.Fatalf("my month: %+v", list)
	}
	var pavelEntry model.AttendanceEntry
	ts.db.Where("contractor_id = ?", pavel.ID).First(&pavelEntry)
	d.mustDo(http.StatusNotFound, "PATCH", path(d, "/me/attendance/%d", pavelEntry.ID), map[string]any{"note": "x"})
	res, body = d.do("POST", d.acct("/me/attendance/actions/submit"), api.AttendanceIDs{IDs: []uint{own.ID, pavelEntry.ID}})
	assertError(t, res, body, http.StatusNotFound, "")

	// submit → not editable for the contractor → manager rejects → editable again → resubmit → approve
	doJSON[api.ListResponse[api.AttendanceEntry]](d, http.StatusOK, "POST", d.acct("/me/attendance/actions/submit"), api.AttendanceIDs{IDs: []uint{own.ID, closed.ID}})
	res, body = d.do("PATCH", path(d, "/me/attendance/%d", own.ID), map[string]any{"note": "x"})
	assertCode(t, res, body, http.StatusConflict, "not_editable")
	res, body = d.do("POST", d.acct("/attendance/actions/approve"), api.AttendanceIDs{IDs: []uint{own.ID}})
	assertError(t, res, body, http.StatusForbidden, "")
	doJSON[api.ListResponse[api.AttendanceEntry]](a, http.StatusOK, "POST", a.acct("/attendance/actions/reject"), api.AttendanceIDs{IDs: []uint{own.ID}, Reason: "špatný čas"})
	fixed := doJSON[api.AttendanceEntry](d, http.StatusOK, "PATCH", path(d, "/me/attendance/%d", own.ID), map[string]any{"end": "10:30"})
	if fixed.Status != "draft" || *fixed.End != "10:30" {
		t.Fatalf("fixed: %+v", fixed)
	}
	doJSON[api.ListResponse[api.AttendanceEntry]](d, http.StatusOK, "POST", d.acct("/me/attendance/actions/submit"), api.AttendanceIDs{IDs: []uint{own.ID}})
	ok := doJSON[api.ListResponse[api.AttendanceEntry]](a, http.StatusOK, "POST", a.acct("/attendance/actions/approve"), api.AttendanceIDs{IDs: []uint{own.ID, closed.ID}})
	if ok.Total != 2 || ok.Items[0].Status != "approved" {
		t.Fatalf("approved: %+v", ok)
	}

	// a staff user without a contractor gets a clear error on /me
	res, body = a.do("GET", a.acct("/me/contractor"), nil)
	assertCode(t, res, body, http.StatusConflict, "not_contractor")
}

// Every account operation that changes data declares its roles, and a
// contractor reaches only /me and the account itself.
func TestRoleDeclarations(t *testing.T) {
	_, hapi := api.New(nil, config.Config{}, api.Deps{})
	prefix := "/api/accounts/{slug}"
	for p, item := range hapi.OpenAPI().Paths {
		if !strings.HasPrefix(p, prefix) {
			continue
		}
		rel := strings.TrimPrefix(p, prefix)
		for method, op := range map[string]*huma.Operation{"GET": item.Get, "POST": item.Post, "PUT": item.Put, "PATCH": item.Patch, "DELETE": item.Delete} {
			if op == nil {
				continue
			}
			roles := auth.AllowedRoles(op)
			if method != "GET" && roles == nil && !(method == "DELETE" && rel == "/members/{user_id}") {
				t.Errorf("%s %s declares no roles", method, p)
			}
			if slices.Contains(roles, model.RoleContractor) && !strings.HasPrefix(rel, "/me/") && rel != "" {
				t.Errorf("%s %s is open to contractors", method, p)
			}
		}
	}
}
