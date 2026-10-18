package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/qwerin/zahrada/internal/api"
	"github.com/qwerin/zahrada/internal/model"
)

// bearerTransport adds the API token to every MCP request.
type bearerTransport struct{ token string }

func (b bearerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	if b.token != "" {
		r.Header.Set("Authorization", "Bearer "+b.token)
	}
	return http.DefaultTransport.RoundTrip(r)
}

// mcpSession connects a real MCP client to the API with token.
func mcpSession(t *testing.T, ts *testServer, token string) *mcp.ClientSession {
	t.Helper()
	srv := httptest.NewServer(ts.handler)
	t.Cleanup(srv.Close)
	cl := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	sess, err := cl.Connect(context.Background(), &mcp.StreamableClientTransport{
		Endpoint: srv.URL + "/api/mcp", HTTPClient: &http.Client{Transport: bearerTransport{token}},
	}, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = sess.Close() })
	return sess
}

// apiToken creates an API token of c's user.
func apiToken(c *client) string {
	return doJSON[api.CreatedAPIToken](c, http.StatusCreated, "POST", "/api/auth/tokens", api.APITokenCreate{Name: "mcp"}).Token
}

// callTool calls an MCP tool and returns its text and error flag.
func callTool(t *testing.T, s *mcp.ClientSession, name string, args map[string]any) (string, bool) {
	t.Helper()
	res, err := s.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	var sb strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			sb.WriteString(tc.Text)
		}
	}
	return sb.String(), res.IsError
}

// mustTool is callTool expecting success, decoding the JSON result into T.
func mustTool[T any](t *testing.T, s *mcp.ClientSession, name string, args map[string]any) T {
	t.Helper()
	text, isErr := callTool(t, s, name, args)
	if isErr {
		t.Fatalf("%s(%v): error %s", name, args, text)
	}
	return decodeJSON[T](t, []byte(text))
}

func TestMCPTools(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	s := mcpSession(t, ts, apiToken(a))

	tools, err := s.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]*mcp.Tool{}
	for _, tl := range tools.Tools {
		byName[tl.Name] = tl
	}
	for _, name := range []string{"list_accounts", "list_jobs", "create_job", "update_quote", "list_price_items", "get_attendance_summary"} {
		if byName[name] == nil {
			t.Fatalf("missing tool %s", name)
		}
	}
	if !byName["list_jobs"].Annotations.ReadOnlyHint || byName["update_quote"].Annotations.ReadOnlyHint {
		t.Fatal("annotations")
	}
	// nothing that deletes, approves, sends or reconfigures
	for name := range byName {
		for _, bad := range []string{"delete", "approve", "close", "invite", "status_quote", "settings"} {
			if strings.Contains(name, bad) {
				t.Fatalf("unsafe tool %s", name)
			}
		}
	}
	schema, _ := json.Marshal(byName["create_attendance"].InputSchema)
	if strings.Contains(string(schema), "rate_snapshot") {
		t.Fatalf("rates must not be settable through MCP: %s", schema)
	}

	// celý postup nabídky: klient → zahrada → zakázka → kalkulace s položkami z ceníku
	acct := func(m map[string]any) map[string]any { m["account"] = a.slug; return m }
	cl := mustTool[api.ClientDetail](t, s, "create_client", acct(map[string]any{"body": map[string]any{"name": "Jana Nováková", "kind": "person"}}))
	site := mustTool[api.Site](t, s, "create_site", acct(map[string]any{"body": map[string]any{"client_id": cl.ID, "name": "Zahrada", "city": "Říčany"}}))
	job := mustTool[api.Job](t, s, "create_job", acct(map[string]any{"body": map[string]any{"client_id": cl.ID, "site_id": site.ID, "kind": "realization", "title": "Trávník"}}))
	items := mustTool[api.ListResponse[api.PriceItem]](t, s, "import_price_items", acct(map[string]any{"body": map[string]any{"items": []map[string]any{
		{"kind": "work", "name": "Frézování", "unit": "m²", "unit_price": 1500, "norm_hours": "0.02"},
	}}}))
	q := mustTool[api.Quote](t, s, "create_quote", acct(map[string]any{"id": job.ID, "body": map[string]any{}}))
	q = mustTool[api.Quote](t, s, "update_quote", acct(map[string]any{"id": q.ID, "body": map[string]any{
		"title": "Založení trávníku", "items": []map[string]any{
			{"kind": "work", "name": "Frézování", "unit": "m²", "quantity": "200", "unit_price": 1500, "vat_rate_bps": 2100, "norm_hours": "0.02", "price_item_id": items.Items[0].ID},
		}}}))
	if q.Net != 300000 || q.Hours != "4" || q.Status != "draft" {
		t.Fatalf("quote: %+v", q.QuoteSummary)
	}
	// the model cannot send or accept the offer
	if byName["change_quote_status"] != nil {
		t.Fatal("quote status must stay in the web app")
	}
	if text, isErr := callTool(t, s, "get_job", acct(map[string]any{"id": 9999})); !isErr || !strings.Contains(text, "HTTP 404") {
		t.Fatalf("missing job: %v %s", isErr, text)
	}
	if text, isErr := callTool(t, s, "get_job", map[string]any{"id": job.ID}); !isErr || !strings.Contains(text, "account") {
		t.Fatalf("missing account: %v %s", isErr, text)
	}
}

// TestMCPIsolationAndRoles: MCP calls are subject to membership and roles like REST.
func TestMCPIsolationAndRoles(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	b := ts.signup("b@example.cz", "Firma B")
	acme := newClient(a, "ACME")

	sb := mcpSession(t, ts, apiToken(b))
	if text, isErr := callTool(t, sb, "get_client", map[string]any{"account": a.slug, "id": acme.ID}); !isErr || !strings.Contains(text, "HTTP 404") {
		t.Fatalf("foreign account: %v %s", isErr, text)
	}
	if text, isErr := callTool(t, sb, "list_clients", map[string]any{"account": "../" + a.slug}); !isErr || !strings.Contains(text, "HTTP 404") {
		t.Fatalf("slug escape: %v %s", isErr, text)
	}
	acc := ts.memberOf(a, "ucetni@example.cz", model.RoleAccountant)
	sa := mcpSession(t, ts, apiToken(acc))
	if text, isErr := callTool(t, sa, "create_client", map[string]any{"account": a.slug, "body": map[string]any{"name": "X", "kind": "person"}}); !isErr || !strings.Contains(text, "HTTP 403") {
		t.Fatalf("accountant write: %v %s", isErr, text)
	}
	if text, isErr := callTool(t, sa, "list_jobs", map[string]any{"account": a.slug}); isErr {
		t.Fatalf("accountant may read: %s", text)
	}
	// a contractor reaches nothing but the account itself
	petr := newContractor(a, "Petr", 30000)
	d := contractorLogin(t, ts, a, petr.ID, "petr@example.cz")
	sd := mcpSession(t, ts, apiToken(d))
	if text, isErr := callTool(t, sd, "list_clients", map[string]any{"account": a.slug}); !isErr || !strings.Contains(text, "HTTP 403") {
		t.Fatalf("contractor: %v %s", isErr, text)
	}
}

// TestMCPAuth: only a valid API token opens the endpoint (no cookies).
func TestMCPAuth(t *testing.T) {
	ts := newTestServer(t, withRateLimit)
	a := ts.signup("a@example.cz", "Firma A")
	init := map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{
		"protocolVersion": "2025-06-18", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "x", "version": "1"},
	}}

	res, body := a.do("POST", "/api/mcp", init) // session cookie only
	if res.StatusCode != http.StatusUnauthorized || !strings.HasPrefix(res.Header.Get("WWW-Authenticate"), "Bearer") {
		t.Fatalf("cookie: %d %s", res.StatusCode, body)
	}
	if s := mcpSession(t, ts, apiToken(a)); s.InitializeResult().ServerInfo.Name != "zahrada" {
		t.Fatalf("initialize: %+v", s.InitializeResult())
	}
	bad := &client{ts: ts, token: "zh_invalid"}
	for i := range 20 {
		if res, body := bad.do("POST", "/api/mcp", init); res.StatusCode != http.StatusUnauthorized {
			t.Fatalf("attempt %d: %d %s", i, res.StatusCode, body)
		}
	}
	if res, _ := bad.do("POST", "/api/mcp", init); res.StatusCode != http.StatusTooManyRequests || res.Header.Get("Retry-After") == "" {
		t.Fatalf("rate limit: %d %v", res.StatusCode, res.Header)
	}
}

// TestMCPEveryTool calls every read tool once (writes are covered above, lookup_ares below).
func TestMCPEveryTool(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	s := mcpSession(t, ts, apiToken(a))
	acct := func(m map[string]any) map[string]any {
		if m == nil {
			m = map[string]any{}
		}
		m["account"] = a.slug
		return m
	}
	cl := newClient(a, "Klient")
	petr := newContractor(a, "Petr", 30000)
	job := newJob(a, api.JobCreate{ClientID: cl.ID, Kind: "realization", Title: "Trávník", Status: "ordered", ContractorIDs: []uint{petr.ID}})
	q := doJSON[api.Quote](a, http.StatusCreated, "POST", path(a, "/jobs/%d/quotes", job.ID), api.QuoteCreate{})
	mustTool[api.AttendanceEntry](t, s, "create_attendance", acct(map[string]any{"body": map[string]any{
		"contractor_id": petr.ID, "job_id": job.ID, "work_on": "2026-03-10", "start": "07:00", "end": "15:00", "break_min": 30, "rate_snapshot": 1,
	}}))
	mustTool[api.Job](t, s, "update_job", acct(map[string]any{"id": job.ID, "body": map[string]any{"note": "Pozor na psa"}}))
	mustTool[api.Job](t, s, "change_job_status", acct(map[string]any{"id": job.ID, "body": map[string]any{"status": "done"}}))
	mustTool[api.ClientDetail](t, s, "update_client", acct(map[string]any{"id": cl.ID, "body": map[string]any{"phone": "777 111 222"}}))
	mustTool[api.Todo](t, s, "create_todo", acct(map[string]any{"body": map[string]any{"text": "Zavolat klientce"}}))

	for name, args := range map[string]map[string]any{
		"list_accounts":            {},
		"get_account":              acct(nil),
		"search":                   acct(map[string]any{"q": "trav"}),
		"list_jobs":                acct(map[string]any{"status": "open"}),
		"get_job":                  acct(map[string]any{"id": job.ID}),
		"list_clients":             acct(nil),
		"get_client":               acct(map[string]any{"id": cl.ID}),
		"list_sites":               acct(nil),
		"list_contractors":         acct(nil),
		"list_contractor_payments": acct(map[string]any{"id": petr.ID}),
		"list_price_items":         acct(nil),
		"list_job_quotes":          acct(map[string]any{"id": job.ID}),
		"get_quote":                acct(map[string]any{"id": q.ID}),
		"list_attendance":          acct(map[string]any{"from": "2026-03-01", "to": "2026-03-31"}),
		"get_attendance_summary":   acct(map[string]any{"month": "2026-03"}),
		"list_todos":               acct(nil),
		"preview_calculation":      acct(map[string]any{"body": map[string]any{"items": []map[string]any{{"kind": "work", "quantity": "2", "unit_price": 100, "vat_rate_bps": 2100}}}}),
	} {
		text, isErr := callTool(t, s, name, args)
		if isErr || !(strings.HasPrefix(text, "{") || strings.HasPrefix(text, "[")) {
			t.Errorf("%s: %v %s", name, isErr, text)
		}
	}
	sum := mustTool[api.AttendanceSummary](t, s, "get_attendance_summary", acct(map[string]any{"month": "2026-03"}))
	// the rate comes from the contractor, never from the model (rate_snapshot is dropped)
	if len(sum.Contractors) != 1 || sum.Contractors[0].Minutes != 450 || sum.Contractors[0].Cost != 225000 {
		t.Fatalf("summary: %+v", sum)
	}
}

func TestMCPLookupARES(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	withARES(ts, fakeARES{"27074358": {RegistrationNo: "27074358", Name: "Firma a.s."}})
	s := mcpSession(t, ts, apiToken(a))
	if text, isErr := callTool(t, s, "lookup_ares", map[string]any{"ico": "27074358"}); isErr || !strings.Contains(text, "Firma a.s.") {
		t.Fatalf("lookup_ares: %v %s", isErr, text)
	}
}
