package api_test

import (
	"net/http"
	"testing"

	"github.com/qwerin/zahrada/internal/api"
	"github.com/qwerin/zahrada/internal/model"
)

func TestAccountDefaults(t *testing.T) {
	ts := newTestServer(t)
	c := ts.signup("a@example.cz", "Firma A")

	acc := doJSON[api.Account](c, http.StatusOK, "GET", c.acct(""), nil)
	if acc.Slug != "firma-a" || acc.Role != "owner" || acc.Country != "CZ" || acc.VatPayer ||
		acc.DefaultVatRateBps != 2100 || acc.AttendanceRoundingMin != 15 || acc.AttendanceRoundingMode != "nearest" ||
		acc.PlanHorizonWeeks != 8 || acc.QuoteValidityDays != 30 || !acc.Capabilities.Staff || !acc.Capabilities.ManageOwners {
		t.Fatalf("defaults: %+v", acc)
	}
}

func TestCreateAndListAccounts(t *testing.T) {
	ts := newTestServer(t)
	c := ts.signup("a@example.cz", "Firma")

	acc := doJSON[api.Account](c, http.StatusCreated, "POST", "/api/accounts", api.AccountCreate{Name: "Firma"})
	if acc.Slug != "firma-2" || acc.Role != "owner" {
		t.Fatalf("created: %+v", acc)
	}
	doJSON[api.Account](c, http.StatusCreated, "POST", "/api/accounts", api.AccountCreate{Name: "Firma"})

	list := doJSON[api.ListResponse[api.Account]](c, http.StatusOK, "GET", "/api/accounts", nil)
	if list.Total != 3 || len(list.Items) != 3 || list.Items[2].Slug != "firma-3" {
		t.Fatalf("list: %+v", list)
	}
	page := doJSON[api.ListResponse[api.Account]](c, http.StatusOK, "GET", "/api/accounts?page=2&per_page=2", nil)
	if page.Total != 3 || len(page.Items) != 1 || page.Page != 2 || page.PerPage != 2 {
		t.Fatalf("page 2: %+v", page)
	}
	res, body := c.do("GET", "/api/accounts?per_page=500", nil)
	assertError(t, res, body, http.StatusUnprocessableEntity, "per_page")

	me := doJSON[api.Me](c, http.StatusOK, "GET", "/api/auth/me", nil)
	if len(me.Accounts) != 3 {
		t.Fatalf("me accounts: %+v", me.Accounts)
	}

	res, body = c.do("POST", "/api/accounts", api.AccountCreate{Name: ""})
	assertError(t, res, body, http.StatusUnprocessableEntity, "name")
}

func TestPatchAccount(t *testing.T) {
	ts := newTestServer(t)
	c := ts.signup("a@example.cz", "Firma A")

	acc := doJSON[api.Account](c, http.StatusOK, "PATCH", c.acct(""), map[string]any{
		"name": "Firma A s.r.o.", "registration_no": "12345678", "vat_payer": true, "default_hourly_rate": 35000,
		"attendance_rounding_min": 30, "attendance_rounding_mode": "up", "time_zone": "Europe/Bratislava", "default_vat_rate_bps": 0,
	})
	if acc.Name != "Firma A s.r.o." || acc.RegistrationNo != "12345678" || !acc.VatPayer || acc.DefaultHourlyRate != 35000 ||
		acc.AttendanceRoundingMin != 30 || acc.AttendanceRoundingMode != "up" || acc.TimeZone != "Europe/Bratislava" ||
		acc.DefaultVatRateBps != 0 || acc.Slug != "firma-a" {
		t.Fatalf("patched: %+v", acc)
	}
	// unchanged fields are kept, false/0 can be set
	acc = doJSON[api.Account](c, http.StatusOK, "PATCH", c.acct(""), map[string]any{"vat_payer": false})
	if acc.VatPayer || acc.DefaultHourlyRate != 35000 || acc.Name != "Firma A s.r.o." {
		t.Fatalf("second patch: %+v", acc)
	}

	for field, value := range map[string]any{"attendance_rounding_min": 7, "country": "cz", "time_zone": "Mars/Base", "name": "   "} {
		res, body := c.do("PATCH", c.acct(""), map[string]any{field: value})
		assertError(t, res, body, http.StatusUnprocessableEntity, field)
	}
}

func TestAccountNonMemberGets404(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	b := ts.signup("b@example.cz", "Firma B")

	for _, method := range []string{"GET", "PATCH"} {
		res, body := b.do(method, a.acct(""), map[string]any{"name": "hacked"})
		assertError(t, res, body, http.StatusNotFound, "account not found")
	}
	res, body := b.do("GET", "/api/accounts/does-not-exist", nil)
	assertError(t, res, body, http.StatusNotFound, "account not found")

	list := doJSON[api.ListResponse[api.Account]](b, http.StatusOK, "GET", "/api/accounts", nil)
	if list.Total != 1 || list.Items[0].Slug != "firma-b" {
		t.Fatalf("b's accounts: %+v", list)
	}
	res, body = ts.anon().do("GET", a.acct(""), nil)
	assertError(t, res, body, http.StatusUnauthorized, "authentication required")
}

func TestMemberCannotPatchAccount(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	b := ts.signup("b@example.cz", "Firma B")

	var acc model.Account
	ts.db.Where("slug = ?", a.slug).First(&acc)
	var user model.User
	ts.db.Where("email = ?", "b@example.cz").First(&user)
	ts.db.Create(&model.Membership{UserID: user.ID, AccountID: acc.ID, Role: model.RoleMember})

	got := doJSON[api.Account](b, http.StatusOK, "GET", a.acct(""), nil)
	if got.Role != "member" {
		t.Fatalf("role = %q", got.Role)
	}
	res, body := b.do("PATCH", a.acct(""), map[string]any{"name": "hacked"})
	assertError(t, res, body, http.StatusForbidden, "allowed roles: owner, admin")
}
