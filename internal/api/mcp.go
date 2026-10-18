package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/qwerin/zahrada/internal/auth"
)

// MCP server (SPEC §7.17): POST /api/mcp speaks the Model Context Protocol
// (Streamable HTTP, stateless) to AI assistants. Every request carries an API
// token (`Authorization: Bearer zh_…`), a personal one or an OAuth access
// token (oauth.go); cookies are not accepted.
//
// A tool is a curated REST operation: its input schema is derived from the
// OpenAPI operation (path/query parameters + request body) and a call is
// dispatched in-process to the API router with the caller's token, so
// authentication, roles, validation, events and rate limits are exactly those
// of the REST API. Only reads and safe writes are exposed (no e-mails,
// deletions, settings).

// mcpPath is where the MCP endpoint is mounted.
const mcpPath = "/api/mcp"

// mcpMaxBody bounds a JSON-RPC message (tool bodies are small JSON documents).
const mcpMaxBody = 1 << 20

// mcpTool describes one MCP tool backed by a REST operation.
type mcpTool struct {
	Name        string
	Title       string
	Description string
	Method      string         // http.MethodGet …
	Path        string         // relative to /api/accounts/{slug} (account tools) or absolute ("/api/…")
	Write       bool           // false = read-only
	Force       map[string]any // body fields always set, whatever the model sends
	Omit        []string       // body fields hidden from the schema and dropped from calls
}

// account tools need the account slug; absolute paths do not.
func (t mcpTool) account() bool { return !strings.HasPrefix(t.Path, "/api/") }

func (t mcpTool) openAPIPath() string {
	if t.account() {
		return "/api/accounts/{slug}" + t.Path
	}
	return t.Path
}

// mcpTools is the catalogue exposed over MCP. Keep descriptions in English
// (models follow them best); the data itself is Czech.
var mcpTools = []mcpTool{
	{Name: "list_accounts", Title: "Účty", Method: http.MethodGet, Path: "/api/accounts",
		Description: "List the accounts (companies) the token's user is a member of, with the user's role. Every other tool takes the `account` slug from here."},
	{Name: "get_account", Title: "Nastavení firmy", Method: http.MethodGet, Path: "",
		Description: "Company details and settings: name, IČO, address, VAT payer, default contractor hourly rate, attendance rounding, quote validity and terms."},
	{Name: "search", Title: "Hledání", Method: http.MethodGet, Path: "/search",
		Description: "Global search across jobs, clients, gardens (sites) and contractors (diacritics-insensitive)."},

	{Name: "list_jobs", Title: "Zakázky", Method: http.MethodGet, Path: "/jobs",
		Description: "List jobs (zakázky) with filters. status=open returns everything not done/invoiced/cancelled; also comma-separated statuses, kind, client_id, contractor_id, query."},
	{Name: "get_job", Title: "Detail zakázky", Method: http.MethodGet, Path: "/jobs/{id}",
		Description: "Job detail: client, garden, kind, status and allowed next statuses, planned dates, assigned contractor IDs, accepted quote."},
	{Name: "create_job", Title: "Nová zakázka", Method: http.MethodPost, Path: "/jobs", Write: true,
		Description: "Create a job for an existing client (and optionally one of the client's gardens). The number is assigned automatically. Confirm with the user first."},
	{Name: "update_job", Title: "Upravit zakázku", Method: http.MethodPatch, Path: "/jobs/{id}", Write: true,
		Description: "Change a job: title, kind, garden (site_id, 0 removes), planned dates, monthly fee, note, assigned contractors (contractor_ids replaces the list). Omitted fields stay unchanged."},
	{Name: "change_job_status", Title: "Změnit stav zakázky", Method: http.MethodPost, Path: "/jobs/{id}/status", Write: true,
		Description: "Move a job to another status; only transitions listed in the job's next_statuses are allowed. Confirm with the user first."},

	{Name: "list_clients", Title: "Klienti", Method: http.MethodGet, Path: "/clients",
		Description: "List clients (customers); query searches name, IČO, e-mail, phone and address."},
	{Name: "get_client", Title: "Detail klienta", Method: http.MethodGet, Path: "/clients/{id}",
		Description: "Client detail including all their gardens (sites) with address, areas and access notes."},
	{Name: "create_client", Title: "Nový klient", Method: http.MethodPost, Path: "/clients", Write: true,
		Description: "Create a client. Use lookup_ares first for companies (IČO). Search existing clients first to avoid duplicates."},
	{Name: "update_client", Title: "Upravit klienta", Method: http.MethodPatch, Path: "/clients/{id}", Write: true,
		Description: "Change fields of a client; omitted fields stay unchanged."},
	{Name: "list_sites", Title: "Zahrady", Method: http.MethodGet, Path: "/sites",
		Description: "List gardens (sites), optionally of one client (client_id)."},
	{Name: "create_site", Title: "Nová zahrada", Method: http.MethodPost, Path: "/sites", Write: true,
		Description: "Add a garden (place of work) to a client: address, lawn and hedge sizes, access note (keys, gate code, dog)."},
	{Name: "lookup_ares", Title: "Hledání v ARES", Method: http.MethodGet, Path: "/api/ares/{ico}",
		Description: "Look up a Czech company in the ARES business register by IČO (8 digits): name, address, DIČ."},

	{Name: "list_contractors", Title: "Dodavatelé", Method: http.MethodGet, Path: "/contractors",
		Description: "List contractors (people working for the firm) with hourly and per-km rates."},

	{Name: "list_price_items", Title: "Ceník", Method: http.MethodGet, Path: "/price-items",
		Description: "Price list: work and material items with unit, purchase cost, sale price (sale_price), markup, waste and labour norm (hours per unit). Use these values when preparing a calculation."},
	{Name: "import_price_items", Title: "Doplnit ceník", Method: http.MethodPost, Path: "/price-items/import", Write: true,
		Description: "Add many price list items at once. Money in haléře (1 Kč = 100). unit_price = sale price per unit; or unit_cost + markup_bps. Confirm the list with the user first."},
	{Name: "preview_calculation", Title: "Spočítat kalkulaci", Method: http.MethodPost, Path: "/calc/preview",
		Description: "Compute a calculation without saving: totals per VAT rate, costs, margin and labour hours. Quantities are decimal strings, money in haléře."},
	{Name: "list_job_quotes", Title: "Kalkulace zakázky", Method: http.MethodGet, Path: "/jobs/{id}/quotes",
		Description: "Versions of the calculation / offer of a job with status and totals."},
	{Name: "get_quote", Title: "Detail kalkulace", Method: http.MethodGet, Path: "/quotes/{id}",
		Description: "Calculation detail: lines, computed totals, client and garden."},
	{Name: "create_quote", Title: "Nová kalkulace", Method: http.MethodPost, Path: "/jobs/{id}/quotes", Write: true,
		Description: "Start a new calculation version of a job (empty, or copy_from an existing version). Then fill it with update_quote."},
	{Name: "update_quote", Title: "Vyplnit kalkulaci", Method: http.MethodPut, Path: "/quotes/{id}", Write: true,
		Description: "Replace title, validity, discount, intro, note and ALL lines of a DRAFT calculation. Lines: kind (work/material/machine/transport/subcontract/other), section, name, unit, quantity (decimal string), unit_price (sale, haléře) or unit_cost + markup_bps, vat_rate_bps, optional waste_bps and norm_hours. Prefer values from list_price_items (pass price_item_id). The user sends the offer to the client in the web app."},

	{Name: "list_attendance", Title: "Docházka", Method: http.MethodGet, Path: "/attendance",
		Description: "Attendance entries with filters from/to (YYYY-MM-DD), contractor_id, job_id, status (draft, submitted, approved, rejected, open)."},
	{Name: "get_attendance_summary", Title: "Souhrn docházky", Method: http.MethodGet, Path: "/attendance/summary",
		Description: "Monthly summary (month=YYYY-MM) of approved attendance per contractor and job: days, minutes, labour cost, km, total to pay; plus the number of entries waiting for approval."},
	{Name: "create_attendance", Title: "Zapsat docházku", Method: http.MethodPost, Path: "/attendance", Write: true,
		Omit:        []string{"rate_snapshot", "km_rate_snapshot"},
		Description: "Record work of a contractor as a manager: contractor_id, job_id (0 = overhead), work_on, start and end (HH:MM), break_min, km, note. Entries are approved unless status says otherwise. Confirm with the user first."},

	{Name: "list_todos", Title: "Úkoly", Method: http.MethodGet, Path: "/todos",
		Description: "To-dos of the account."},
	{Name: "create_todo", Title: "Nový úkol", Method: http.MethodPost, Path: "/todos", Write: true,
		Description: "Create a to-do, optionally related to a job, client, garden or contractor."},
}

const mcpInstructions = `Zahrada is an app of a Czech garden-services company: jobs (zakázky) for clients and their gardens, calculations that become offers, a price list, contractors and their attendance.
Money is in haléře (integer, 1 Kč = 100); quantities are decimal strings ("12.5"); percentages are basis points (2100 = 21 %); dates YYYY-MM-DD; attendance times HH:MM.
Every account tool needs the account slug: call list_accounts first.
To prepare an offer: find or create the client (and garden), create the job, create_quote, then update_quote with lines (use list_price_items for prices and labour norms). The user reviews, sends and prints the offer in the web app.
Writing tools create real records – confirm with the user first.
Nothing is ever deleted, sent to clients, approved or reconfigured through these tools; point the user to the web app for that.`

// mcpReq carries what the dispatcher needs from the outer HTTP request.
type mcpReq struct {
	remoteAddr string
	header     http.Header
}

type mcpReqKey struct{}

// mcpHandler builds the MCP endpoint dispatching tool calls to router.
func (s *server) mcpHandler(router http.Handler, api huma.API) http.Handler {
	srv := mcp.NewServer(&mcp.Implementation{Name: "zahrada", Title: "Zahrada", Version: "1.0.0"},
		&mcp.ServerOptions{Instructions: mcpInstructions})
	oapi := api.OpenAPI()
	for _, t := range mcpTools {
		srv.AddTool(t.tool(oapi), s.mcpCall(router, t))
	}
	h := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, &mcp.StreamableHTTPOptions{
		Stateless: true, JSONResponse: true,
		// DNS rebinding cannot steal anything: every request needs a bearer token,
		// and the check would reject reverse proxies on localhost.
		DisableLocalhostProtection: true,
	})
	return s.mcpAuth(h)
}

// mcpAuth requires a valid API token (401 with WWW-Authenticate otherwise;
// failed attempts are rate limited per IP) and remembers the request for the
// dispatcher.
func (s *server) mcpAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		unauthorized := func(msg string) {
			// RFC 9728: where clients find the OAuth server (SPEC §7.17)
			w.Header().Set("WWW-Authenticate", `Bearer realm="zahrada", resource_metadata="`+
				s.oauthIssuer()+"/.well-known/oauth-protected-resource"+mcpPath+`"`)
			http.Error(w, msg, http.StatusUnauthorized)
		}
		ip := clientIP(r.Context())
		if err := s.rateBlocked(s.limits.mcpAuth, ip); err != nil {
			w.Header().Set("Retry-After", retryAfter(err))
			http.Error(w, "too many failed attempts", http.StatusTooManyRequests)
			return
		}
		token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok {
			unauthorized("API token required (Authorization: Bearer zh_…)")
			return
		}
		if _, err := s.auth.UserByAPIToken(r.Context(), token); err != nil {
			if !errors.Is(err, auth.ErrInvalidCredentials) {
				slog.ErrorContext(r.Context(), "mcp token check", "err", err)
				http.Error(w, "authentication failed", http.StatusInternalServerError)
				return
			}
			s.rateFail(s.limits.mcpAuth, ip)
			unauthorized("invalid or expired API token")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, mcpMaxBody)
		ctx := context.WithValue(r.Context(), mcpReqKey{}, mcpReq{remoteAddr: r.RemoteAddr, header: r.Header.Clone()})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// retryAfter extracts the Retry-After header of a rate limit error ("60" fallback).
func retryAfter(err error) string {
	var he huma.HeadersError
	if errors.As(err, &he) {
		if v := he.GetHeaders().Get("Retry-After"); v != "" {
			return v
		}
	}
	return "60"
}

var mcpPathParam = regexp.MustCompile(`\{([a-z_]+)\}`)

// tool builds the MCP tool definition from the OpenAPI operation.
func (t mcpTool) tool(oapi *huma.OpenAPI) *mcp.Tool {
	op := mcpOperation(oapi, t.Method, t.openAPIPath())
	props := map[string]any{}
	var required []string
	if t.account() {
		props["account"] = map[string]any{"type": "string", "description": "Account slug (see list_accounts)"}
		required = append(required, "account")
	}
	defs := map[string]any{}
	for _, p := range op.Parameters {
		if p.Name == "slug" || (p.In != "path" && p.In != "query") {
			continue
		}
		sch := mcpSchemaJSON(p.Schema, oapi, defs)
		if p.Description != "" {
			sch["description"] = p.Description
		}
		props[p.Name] = sch
		if p.Required || p.In == "path" {
			required = append(required, p.Name)
		}
	}
	if rb := op.RequestBody; rb != nil {
		if c := rb.Content["application/json"]; c != nil && c.Schema != nil {
			props["body"] = mcpOmit(mcpSchemaJSON(c.Schema, oapi, defs), defs, t.Omit)
			required = append(required, "body")
		}
	}
	in := map[string]any{"type": "object", "properties": props, "additionalProperties": false}
	if len(required) > 0 {
		in["required"] = required
	}
	if len(defs) > 0 {
		in["$defs"] = defs
	}
	desc := t.Description
	if roles := auth.AllowedRoles(op); len(roles) > 0 {
		desc += " Roles: " + strings.Join(roles, ", ") + "."
	}
	return &mcp.Tool{
		Name: t.Name, Title: t.Title, Description: desc, InputSchema: in,
		Annotations: &mcp.ToolAnnotations{
			Title: t.Title, ReadOnlyHint: !t.Write, IdempotentHint: !t.Write,
			DestructiveHint: new(false), OpenWorldHint: new(t.Name == "lookup_ares"),
		},
	}
}

// mcpOmit removes fields from a body schema: a referenced component is
// inlined without them.
func mcpOmit(body, defs map[string]any, fields []string) map[string]any {
	if len(fields) == 0 {
		return body
	}
	if ref, ok := body["$ref"].(string); ok {
		name := strings.TrimPrefix(ref, "#/$defs/")
		body = maps.Clone(defs[name].(map[string]any))
		delete(defs, name) // the request body component is not referenced elsewhere
	}
	props := maps.Clone(body["properties"].(map[string]any))
	for _, f := range fields {
		delete(props, f)
	}
	body["properties"] = props
	if req, ok := body["required"].([]any); ok {
		body["required"] = slices.DeleteFunc(slices.Clone(req), func(v any) bool { return slices.Contains(fields, v.(string)) })
	}
	return body
}

// mcpOperation finds a registered operation; the catalogue must match the API.
func mcpOperation(oapi *huma.OpenAPI, method, path string) *huma.Operation {
	if pi := oapi.Paths[path]; pi != nil {
		var op *huma.Operation
		switch method {
		case http.MethodGet:
			op = pi.Get
		case http.MethodPost:
			op = pi.Post
		case http.MethodPatch:
			op = pi.Patch
		case http.MethodPut:
			op = pi.Put
		}
		if op != nil {
			return op
		}
	}
	panic(fmt.Sprintf("mcp: no operation %s %s", method, path))
}

// mcpSchemaJSON converts a huma schema to plain JSON Schema, moving the
// referenced components (transitively) into defs as "#/$defs/Name".
func mcpSchemaJSON(sch *huma.Schema, oapi *huma.OpenAPI, defs map[string]any) map[string]any {
	const prefix = "#/components/schemas/"
	b, err := json.Marshal(sch)
	if err != nil {
		panic(err)
	}
	var out map[string]any
	if err := json.Unmarshal(bytes.ReplaceAll(b, []byte(prefix), []byte("#/$defs/")), &out); err != nil {
		panic(err)
	}
	for _, ref := range mcpRefs(out) {
		if _, done := defs[ref]; done {
			continue
		}
		comp := oapi.Components.Schemas.Map()[ref]
		if comp == nil {
			panic("mcp: unknown schema " + ref)
		}
		defs[ref] = nil // cycle guard
		defs[ref] = mcpSchemaJSON(comp, oapi, defs)
	}
	return out
}

// mcpRefs lists the $defs names referenced anywhere in v.
func mcpRefs(v any) []string {
	var refs []string
	switch x := v.(type) {
	case map[string]any:
		if r, ok := x["$ref"].(string); ok {
			refs = append(refs, strings.TrimPrefix(r, "#/$defs/"))
		}
		for _, k := range slices.Sorted(maps.Keys(x)) {
			refs = append(refs, mcpRefs(x[k])...)
		}
	case []any:
		for _, e := range x {
			refs = append(refs, mcpRefs(e)...)
		}
	}
	return refs
}

// mcpCall dispatches a tool call as a REST request through router.
func (s *server) mcpCall(router http.Handler, t mcpTool) mcp.ToolHandler {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := map[string]any{}
		if len(req.Params.Arguments) > 0 {
			if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
				return mcpError("arguments must be a JSON object"), nil
			}
		}
		path := t.Path
		if t.account() {
			slug, _ := args["account"].(string)
			if slug == "" {
				return mcpError(`missing "account" (account slug, see list_accounts)`), nil
			}
			path = "/api/accounts/" + url.PathEscape(slug) + path
			delete(args, "account")
		}
		var missing []string
		path = mcpPathParam.ReplaceAllStringFunc(path, func(m string) string {
			name := m[1 : len(m)-1]
			v, ok := args[name]
			delete(args, name)
			if !ok {
				missing = append(missing, name)
				return m
			}
			return url.PathEscape(mcpScalar(v))
		})
		if len(missing) > 0 {
			return mcpError("missing " + strings.Join(missing, ", ")), nil
		}
		var body io.Reader = http.NoBody
		if b, ok := args["body"]; ok {
			delete(args, "body")
			if obj, ok := b.(map[string]any); ok {
				for _, f := range t.Omit {
					delete(obj, f)
				}
				maps.Copy(obj, t.Force)
			}
			raw, err := json.Marshal(b)
			if err != nil {
				return mcpError("invalid body"), nil
			}
			body = bytes.NewReader(raw)
		}
		q := url.Values{}
		for _, k := range slices.Sorted(maps.Keys(args)) {
			if args[k] != nil {
				q.Set(k, mcpScalar(args[k]))
			}
		}
		if len(q) > 0 {
			path += "?" + q.Encode()
		}

		rec, err := mcpDo(ctx, router, req, t.Method, path, body)
		if err != nil {
			return mcpError("invalid request: " + err.Error()), nil
		}
		return mcpResult(rec), nil
	}
}

// mcpDo dispatches one REST request through router with the credentials of
// the MCP request.
func mcpDo(ctx context.Context, router http.Handler, req *mcp.CallToolRequest, method, path string, body io.Reader) (*mcpRecorder, error) {
	// a fresh chi routing context: the outer one belongs to /api/mcp
	r, err := http.NewRequestWithContext(context.WithValue(ctx, chi.RouteCtxKey, nil), method, path, body)
	if err != nil {
		return nil, err
	}
	if outer, ok := ctx.Value(mcpReqKey{}).(mcpReq); ok {
		r.RemoteAddr = outer.remoteAddr
		for k, v := range outer.header { // forwarded headers (client IP), Authorization
			r.Header[k] = v
		}
	} else if req.Extra != nil {
		r.Header = req.Extra.Header.Clone()
	}
	r.Header.Del("Cookie") // the token is the only credential
	r.Header.Del("Content-Length")
	r.Header.Set("Accept", "application/json")
	r.Header.Set("Content-Type", "application/json")
	rec := &mcpRecorder{header: http.Header{}, status: http.StatusOK}
	router.ServeHTTP(rec, r)
	return rec, nil
}

func mcpScalar(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(x)
	default:
		b, _ := json.Marshal(x)
		return string(b)
	}
}

func mcpError(msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: msg}}}
}

// mcpResult turns the REST response into a tool result: the JSON body as
// text (and structured content for objects); errors keep the problem+json.
func mcpResult(rec *mcpRecorder) *mcp.CallToolResult {
	text := strings.TrimSpace(rec.body.String())
	if rec.status >= 400 {
		return mcpError(fmt.Sprintf("HTTP %d: %s", rec.status, text))
	}
	if text == "" {
		text = http.StatusText(rec.status)
	}
	res := &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}
	var obj map[string]any
	if json.Unmarshal(rec.body.Bytes(), &obj) == nil {
		res.StructuredContent = obj
	}
	return res
}

// mcpRecorder buffers the in-process REST response.
type mcpRecorder struct {
	header http.Header
	status int
	body   bytes.Buffer
	wrote  bool
}

func (r *mcpRecorder) Header() http.Header { return r.header }

func (r *mcpRecorder) WriteHeader(code int) {
	if !r.wrote {
		r.status, r.wrote = code, true
	}
}

func (r *mcpRecorder) Write(b []byte) (int, error) {
	r.wrote = true
	return r.body.Write(b)
}
