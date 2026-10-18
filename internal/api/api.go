// Package api registers the HTTP API (huma v2 on chi).
//
// # Route groups and authentication
//
// New builds three huma groups; every operation is registered into exactly one:
//
//   - public  — no authentication (health, auth status, register, login, logout).
//   - authed  — auth.RequireUser middleware: 401 unless the request carries a valid
//     zh_session cookie or `Authorization: Bearer zh_…` token; auth.UserFrom(ctx)
//     is then non-nil.
//   - admin   — authed + prefix "/api/admin" + requireInstanceAdmin: only
//     instance administrators (ZAHRADA_ADMIN_EMAILS with a verified
//     e-mail); others get 403 not_instance_admin (SPEC §3.2).
//   - account — authed + prefix "/api/accounts/{slug}" + auth.RequireAccount: the
//     slug is resolved to an account the user is a member of (non-member → 404);
//     auth.AccountFrom(ctx) and auth.RoleFrom(ctx) are then set. The {slug} path
//     parameter is added to the OpenAPI operation automatically, handler inputs
//     do not declare it.
//
// Membership is thus enforced centrally by the group, never per handler.
// Roles likewise: account operations declare the allowed roles at
// registration (auth.ForEditors, auth.ForManagers, auth.Allow(...)) and
// RequireAccount answers 403 to other roles; undeclared = every member.
//
// # Adding a resource
//
// Create <resource>.go with a `func (s *server) register<Resource>(g huma.API)`
// method that calls huma.Get/Post/Patch/Delete on g with paths relative to the
// group (e.g. "/subjects/{id}"), and call it from New. Query domain data only
// through s.scoped(ctx) or tx.Scopes(inAccount(ctx)) so every query is filtered
// by account_id. DTOs live next to the handlers (see accounts.go for the
// naming: Resource, ResourceCreate, ResourcePatch, toResource).
package api

import (
	"context"
	"crypto/tls"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"github.com/go-webauthn/webauthn/webauthn"
	"gorm.io/gorm"

	"github.com/qwerin/zahrada/internal/ares"
	"github.com/qwerin/zahrada/internal/auth"
	"github.com/qwerin/zahrada/internal/config"
	"github.com/qwerin/zahrada/internal/httpsec"
	"github.com/qwerin/zahrada/internal/mail"
	"github.com/qwerin/zahrada/internal/maildiag"
	"github.com/qwerin/zahrada/internal/scheduler"
	"github.com/qwerin/zahrada/internal/secret"
	"github.com/qwerin/zahrada/internal/storage"
)

// Deps holds swappable dependencies (tests replace them).
type Deps struct {
	Now  func() time.Time // defaults to time.Now
	ARES ARES             // defaults to ares.New(cfg.AresURL)

	Mailer  mail.Mailer     // defaults to SMTP (cfg.SMTPHost) or a log mailer
	Storage storage.Storage // defaults to storage.NewLocal(cfg.DataDir/attachments)
	Secrets *secret.Box     // defaults to a random per-process key

	// Resolver and SMTPTLSConfig are used by the e-mail diagnosis (tests inject fakes).
	Resolver      maildiag.Resolver
	SMTPTLSConfig *tls.Config
}

// ARES looks up subjects in the Czech business register (see internal/ares
// for the error contract: ares.ErrInvalidICO, ares.ErrNotFound, other = unavailable).
type ARES interface {
	Lookup(ctx context.Context, ico string) (*ares.Result, error)
}

type server struct {
	db   *gorm.DB
	cfg  config.Config
	deps Deps
	auth *auth.Service

	webauthn    *webauthn.WebAuthn // security keys (SPEC §3.1); nil with webauthnErr on a bad PUBLIC_URL
	webauthnErr error
	limits      limits
	started     time.Time // process start (admin status uptime)
}

// New builds the API router. db may be nil when only the OpenAPI document is needed.
func New(db *gorm.DB, cfg config.Config, deps Deps) (http.Handler, huma.API) {
	s := newServer(db, cfg, deps)

	router := chi.NewRouter()
	// client IP / HTTPS behind trusted proxies, security headers, CSRF (SPEC §5)
	secOpts := httpsec.Options{TrustedProxies: cfg.TrustedProxies, PublicHTTPS: cfg.PublicHTTPS()}
	router.Use(httpsec.Middleware(secOpts))
	router.Use(httpsec.CSRF)
	hc := huma.DefaultConfig("Zahrada API", "1.0.0")
	hc.OpenAPIPath = "/api/openapi"
	hc.DocsPath = "/api/docs"
	hc.SchemasPath = "/api/schemas"
	if cfg.DisableAPIDocs {
		hc.OpenAPIPath, hc.DocsPath, hc.SchemasPath = "", "", ""
	}
	hc.CreateHooks = nil // no "$schema" field in response bodies
	hc.Components.SecuritySchemes = map[string]*huma.SecurityScheme{
		"session": {Type: "apiKey", In: "cookie", Name: auth.SessionCookie},
		"bearer":  {Type: "http", Scheme: "bearer"},
	}
	api := humachi.New(router, hc)
	// every operation sees the API clock (record(), automatic todos)
	api.UseMiddleware(func(hctx huma.Context, next func(huma.Context)) {
		next(huma.WithContext(hctx, withClock(hctx.Context(), s.deps.Now)))
	})

	public := huma.NewGroup(api)

	authed := huma.NewGroup(api)
	authed.UseMiddleware(s.auth.RequireUser(api))
	authed.UseSimpleModifier(func(o *huma.Operation) {
		o.Security = []map[string][]string{{"session": {}}, {"bearer": {}}}
	})

	account := huma.NewGroup(authed, "/api/accounts/{slug}")
	account.UseMiddleware(s.auth.RequireAccount(api))
	account.UseSimpleModifier(addSlugParam)

	admin := huma.NewGroup(authed, "/api/admin")
	admin.UseMiddleware(s.requireInstanceAdmin(api))

	registerHealth(public)
	s.registerAuth(public, authed)
	s.registerPasswordReset(public)
	s.registerEmailVerification(public, authed)
	s.registerAdmin(admin)
	s.registerTwoFactor(public, authed)
	s.registerTokens(authed)
	s.registerOAuth(authed)
	s.registerAccounts(authed, account)
	s.registerAres(authed)
	s.registerMembers(public, authed, account)
	s.registerAttachments(account)
	s.registerEvents(account)
	s.registerTodos(account)
	s.registerSearch(account)
	s.registerCalc(account)
	s.registerDirectory(account)
	s.registerJobs(account)
	s.registerAttendance(account)
	s.registerQuotes(account)
	s.registerPayments(account)
	s.registerLoginLinks(public, account)

	router.Handle(mcpPath, s.mcpHandler(router, api))

	// OAuth metadata, client registration and token endpoint skip CSRF (oauth.go)
	return s.withOAuth(router, secOpts), api
}

// newServer applies the dependency defaults (shared by New and Jobs).
func newServer(db *gorm.DB, cfg config.Config, deps Deps) *server {
	if deps.Now == nil {
		deps.Now = time.Now
	}
	if deps.ARES == nil {
		deps.ARES = ares.New(cfg.AresURL)
	}
	if deps.Mailer == nil {
		deps.Mailer = defaultMailer(cfg)
	}
	if deps.Storage == nil {
		dir := cfg.DataDir
		if dir == "" {
			dir = "./data"
		}
		deps.Storage = storage.NewLocal(filepath.Join(dir, "attachments"))
	}
	if deps.Secrets == nil {
		deps.Secrets = secret.NewRandom()
	}
	s := &server{db: db, cfg: cfg, deps: deps, auth: auth.NewService(db, deps.Now, secureCookies(cfg)),
		limits: newLimits(deps.Now), started: time.Now()}
	s.webauthn, s.webauthnErr = newWebAuthn(s.publicURL(), cfg.WebAuthnOrigins)
	return s
}

// secureCookies decides the Secure attribute of the session cookie:
// ZAHRADA_SECURE_COOKIES when set, otherwise on for an https PublicURL or
// a request that arrived over HTTPS (directly or via a trusted proxy).
func secureCookies(cfg config.Config) func(context.Context) bool {
	return func(ctx context.Context) bool {
		if cfg.SecureCookies != nil {
			return *cfg.SecureCookies
		}
		return cfg.PublicHTTPS() || httpsec.InfoFrom(ctx).HTTPS
	}
}

func defaultMailer(cfg config.Config) mail.Mailer {
	if cfg.SMTPHost == "" {
		return mail.NewLogMailer(os.Stdout, cfg.MailFrom)
	}
	return mail.NewSMTP(cfg.SMTP()) // DKIM-signed when configured
}

// addSlugParam documents the {slug} path parameter of account-scoped operations.
func addSlugParam(o *huma.Operation) {
	for _, p := range o.Parameters {
		if p.In == "path" && p.Name == "slug" {
			return
		}
	}
	o.Parameters = append([]*huma.Param{{
		Name: "slug", In: "path", Required: true, Schema: &huma.Schema{Type: "string"},
		Description: "Account slug",
	}}, o.Parameters...)
}

// Jobs returns the background jobs (cmd/server registers them in the scheduler).
func Jobs(db *gorm.DB, cfg config.Config, deps Deps) []scheduler.Job {
	s := newServer(db, cfg, deps)
	return []scheduler.Job{
		{Name: "auth-cleanup", Run: s.RunAuthCleanup, Every: time.Hour},
	}
}
