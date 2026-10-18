package api_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/qwerin/zahrada/internal/api"
	"github.com/qwerin/zahrada/internal/auth"
	"github.com/qwerin/zahrada/internal/model"
)

// useLink opens a login link as a fresh browser and returns the signed-in client.
func useLink(t *testing.T, ts *testServer, url string, slug string) (*client, *http.Response, []byte) {
	t.Helper()
	token := url[strings.LastIndex(url, "/")+1:]
	res, body := ts.anon().do("POST", "/api/auth/login-link", api.LoginLinkRequest{Token: token})
	c := &client{ts: ts, slug: slug}
	for _, ck := range res.Cookies() {
		if ck.Name == auth.SessionCookie {
			c.session = ck.Value
		}
	}
	return c, res, body
}

func TestContractorLoginLink(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	cl := newClient(a, "Klient")
	pavel := doJSON[api.Contractor](a, http.StatusCreated, "POST", a.acct("/contractors"), api.ContractorCreate{Name: "Pavel Bez Mailu", Kind: "person"})
	if pavel.Login != "none" {
		t.Fatalf("login before: %+v", pavel)
	}
	job := newJob(a, api.JobCreate{ClientID: cl.ID, Kind: "realization", Title: "Trávník", Status: "ordered", ContractorIDs: []uint{pavel.ID}})

	link := doJSON[api.LoginLinkOut](a, http.StatusCreated, "POST", path(a, "/contractors/%d/login-link", pavel.ID), nil)
	if !strings.Contains(link.URL, "/prihlaseni/") || !link.ExpiresAt.After(ts.now.Add(6*24*time.Hour)) {
		t.Fatalf("link: %+v", link)
	}
	if got := doJSON[api.Contractor](a, http.StatusOK, "GET", path(a, "/contractors/%d", pavel.ID), nil); got.Login != "link" || got.UserEmail != "" {
		t.Fatalf("contractor login: %+v", got)
	}
	members := doJSON[api.ListResponse[api.Member]](a, http.StatusOK, "GET", a.acct("/members"), nil)
	found := false
	for _, m := range members.Items {
		if m.Role == "contractor" {
			found = m.LinkOnly && m.Email == "" && m.Name == "Pavel Bez Mailu"
		}
	}
	if !found {
		t.Fatalf("members: %+v", members.Items)
	}

	d, res, body := useLink(t, ts, link.URL, a.slug)
	if res.StatusCode != http.StatusOK || d.session == "" {
		t.Fatalf("use link: %d %s", res.StatusCode, body)
	}
	me := decodeJSON[api.Me](t, body)
	if !me.User.LinkOnly || me.User.Email != "" || len(me.Accounts) != 1 || me.Accounts[0].Role != "contractor" {
		t.Fatalf("me: %+v", me)
	}
	jobs := doJSON[[]api.MeJob](d, http.StatusOK, "GET", d.acct("/me/jobs"), nil)
	if len(jobs) != 1 || jobs[0].ID != job.ID {
		t.Fatalf("my jobs: %+v", jobs)
	}
	d.mustDo(http.StatusForbidden, "GET", d.acct("/clients"), nil)

	// jednorázový: podruhé neprojde
	_, res, body = useLink(t, ts, link.URL, a.slug)
	assertCode(t, res, body, http.StatusBadRequest, "invalid_link")

	// nový odkaz odhlásí předchozí zařízení
	link2 := doJSON[api.LoginLinkOut](a, http.StatusCreated, "POST", path(a, "/contractors/%d/login-link", pavel.ID), nil)
	d.mustDo(http.StatusUnauthorized, "GET", "/api/auth/me", nil)
	if _, res, _ := useLink(t, ts, link2.URL, a.slug); res.StatusCode != http.StatusOK {
		t.Fatalf("second link: %d", res.StatusCode)
	}
	// propadlý odkaz
	link3 := doJSON[api.LoginLinkOut](a, http.StatusCreated, "POST", path(a, "/contractors/%d/login-link", pavel.ID), nil)
	ts.now = ts.now.Add(8 * 24 * time.Hour)
	_, res, body = useLink(t, ts, link3.URL, a.slug)
	assertCode(t, res, body, http.StatusBadRequest, "invalid_link")

	// heslo ani obnova hesla pro uživatele bez e-mailu neexistují
	var u model.User
	ts.db.Where("link_only = ?", true).First(&u)
	before, _ := ts.mail.Last()
	ts.anon().mustDo(http.StatusNoContent, "POST", "/api/auth/password-reset", map[string]any{"email": u.Email})
	if after, _ := ts.mail.Last(); after.Subject != before.Subject || strings.Join(after.To, ",") != strings.Join(before.To, ",") {
		t.Fatalf("password reset e-mail sent to a placeholder address: %+v", after)
	}

	// vedoucí odkaz nevytvoří; dodavatel s e-mailovým účtem odkaz nedostane
	m := ts.memberOf(a, "vedouci@example.cz", model.RoleMember)
	m.mustDo(http.StatusForbidden, "POST", path(m, "/contractors/%d/login-link", pavel.ID), nil)
	petr := newContractor(a, "Petr", 30000)
	contractorLogin(t, ts, a, petr.ID, "petr@example.cz")
	res, body = a.do("POST", path(a, "/contractors/%d/login-link", petr.ID), nil)
	assertCode(t, res, body, http.StatusConflict, "already_linked")

	// smazání dodavatele smaže i jeho přihlášení bez e-mailu
	a.mustDo(http.StatusOK, "PATCH", path(a, "/jobs/%d", job.ID), map[string]any{"contractor_ids": []uint{}})
	a.mustDo(http.StatusNoContent, "DELETE", path(a, "/contractors/%d", pavel.ID), nil)
	var n int64
	ts.db.Model(&model.User{}).Where("link_only = ?", true).Count(&n)
	if n != 0 {
		t.Fatalf("link-only user left behind: %d", n)
	}
}
