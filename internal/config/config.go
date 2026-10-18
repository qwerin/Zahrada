// Package config loads application configuration from environment variables (SPEC §5).
package config

import (
	"crypto"
	"fmt"
	"net/netip"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/qwerin/zahrada/internal/httpsec"
	"github.com/qwerin/zahrada/internal/mail"
)

// Config holds all runtime settings.
type Config struct {
	ListenAddr  string // ZAHRADA_LISTEN_ADDR
	DBDriver    string // ZAHRADA_DB_DRIVER: sqlite | postgres
	DBDSN       string // ZAHRADA_DB_DSN
	StaticDir   string // ZAHRADA_STATIC_DIR: SPA build directory, empty = API only
	AllowSignup bool   // ZAHRADA_ALLOW_SIGNUP: allow registration when users already exist
	// SecureCookies (ZAHRADA_SECURE_COOKIES): nil = auto (Secure when
	// PublicURL is https or the request arrived over HTTPS); true/false force it.
	SecureCookies *bool
	AresURL       string // ZAHRADA_ARES_URL: ARES base URL override (tests)
	PublicURL     string // ZAHRADA_PUBLIC_URL: external base URL for links in e-mails (no trailing slash)
	DataDir       string // ZAHRADA_DATA_DIR: attachments etc. (default ./data)

	SMTPHost     string // ZAHRADA_SMTP_HOST: empty = e-mails are only logged to stdout
	SMTPPort     int    // ZAHRADA_SMTP_PORT (default 587, or 465 with TLS=tls)
	SMTPUser     string // ZAHRADA_SMTP_USER: empty = no AUTH
	SMTPPassword string // ZAHRADA_SMTP_PASSWORD
	SMTPTLS      string // ZAHRADA_SMTP_TLS: starttls (default) | tls (implicit) | none
	MailFrom     string // ZAHRADA_MAIL_FROM: sender address, e.g. "Zahrada <zahrada@example.cz>"
	TimeZone     string // ZAHRADA_TIMEZONE: default zone of accounts (attendance, plan), default Europe/Prague
	SecretKey    string // ZAHRADA_SECRET_KEY: 32-byte key (base64/hex) encrypting stored secrets; empty = DataDir/secret.key

	// DBLog (ZAHRADA_DB_LOG, default "error"): SQL logging — silent|error|warn|info.
	// warn adds slow queries (≥ DBSlowMS), info every query; values are never logged.
	DBLog string
	// DBSlowMS (ZAHRADA_DB_SLOW_MS, default 1000): slow-query threshold for DBLog=warn.
	DBSlowMS int

	// WebhooksAllowPrivate (ZAHRADA_WEBHOOKS_ALLOW_PRIVATE) lets webhooks call private,
	// loopback and link-local addresses (SSRF protection off; LAN setups, tests).
	WebhooksAllowPrivate bool

	// WebAuthnOrigins (ZAHRADA_WEBAUTHN_ORIGINS, comma-separated) are origins
	// accepted for security keys besides the PublicURL origin (dev: the Vite server).
	WebAuthnOrigins []string

	// TrustedProxies (ZAHRADA_TRUSTED_PROXIES): comma separated IPs/CIDRs of
	// reverse proxies whose X-Forwarded-For / X-Forwarded-Proto are believed
	// (client IP for rate limits, HTTPS detection). Default none.
	TrustedProxies []netip.Prefix

	// SetupToken (ZAHRADA_SETUP_TOKEN): when set, the first registration of
	// an empty instance must present it (protects a fresh deployment).
	SetupToken string

	// DisableRateLimit (ZAHRADA_DISABLE_RATE_LIMIT) turns the in-process
	// rate limits off (load tests, trusted single-user setups).
	DisableRateLimit bool

	// DisableAPIDocs (ZAHRADA_DISABLE_API_DOCS) hides /api/docs,
	// /api/openapi.json and /api/schemas (public by default).
	DisableAPIDocs bool

	// AdminEmails (ZAHRADA_ADMIN_EMAILS, comma-separated, lowercased):
	// instance administrators. A user is an admin only with a listed AND
	// verified e-mail (SPEC §3.2).
	AdminEmails []string

	// DKIM signing of outgoing mail (all three or none): ZAHRADA_DKIM_DOMAIN,
	// ZAHRADA_DKIM_SELECTOR and ZAHRADA_DKIM_PRIVATE_KEY (PEM) or
	// ZAHRADA_DKIM_PRIVATE_KEY_FILE. DKIMKey is the parsed key (never logged).
	DKIMDomain   string
	DKIMSelector string
	DKIMKey      crypto.Signer
}

// IsAdminEmail reports whether email is listed in ZAHRADA_ADMIN_EMAILS
// (verification is checked by the caller).
func (c Config) IsAdminEmail(email string) bool {
	return slices.Contains(c.AdminEmails, strings.ToLower(strings.TrimSpace(email)))
}

// DKIM returns the signing configuration, nil when DKIM is off.
func (c Config) DKIM() *mail.DKIMConfig {
	if c.DKIMKey == nil {
		return nil
	}
	return &mail.DKIMConfig{Domain: c.DKIMDomain, Selector: c.DKIMSelector, Key: c.DKIMKey}
}

// SMTP returns the mailer configuration (host empty = no SMTP).
func (c Config) SMTP() mail.SMTPConfig {
	return mail.SMTPConfig{
		Host: c.SMTPHost, Port: c.SMTPPort, Username: c.SMTPUser, Password: c.SMTPPassword,
		TLS: c.SMTPTLS, From: c.MailFrom, DKIM: c.DKIM(),
	}
}

// PublicHTTPS reports whether PublicURL is an https URL.
func (c Config) PublicHTTPS() bool {
	return strings.HasPrefix(strings.ToLower(c.PublicURL), "https://")
}

// Load reads the configuration from the environment and applies defaults.
func Load() (Config, error) {
	cfg := Config{
		ListenAddr: env("ZAHRADA_LISTEN_ADDR", ":8080"),
		DBDriver:   env("ZAHRADA_DB_DRIVER", "sqlite"),
		DBDSN:      env("ZAHRADA_DB_DSN", "zahrada.db"),
		StaticDir:  os.Getenv("ZAHRADA_STATIC_DIR"),
		AresURL:    os.Getenv("ZAHRADA_ARES_URL"),
		PublicURL:  strings.TrimRight(env("ZAHRADA_PUBLIC_URL", "http://localhost:8080"), "/"),
		DataDir:    env("ZAHRADA_DATA_DIR", "./data"),

		SMTPHost:     os.Getenv("ZAHRADA_SMTP_HOST"),
		SMTPUser:     os.Getenv("ZAHRADA_SMTP_USER"),
		SMTPPassword: os.Getenv("ZAHRADA_SMTP_PASSWORD"),
		SMTPTLS:      env("ZAHRADA_SMTP_TLS", "starttls"),
		MailFrom:     env("ZAHRADA_MAIL_FROM", "Zahrada <zahrada@localhost>"),
		SecretKey:    os.Getenv("ZAHRADA_SECRET_KEY"),
		TimeZone:     env("ZAHRADA_TIMEZONE", "Europe/Prague"),
	}
	for _, o := range strings.Split(os.Getenv("ZAHRADA_WEBAUTHN_ORIGINS"), ",") {
		if o = strings.TrimRight(strings.TrimSpace(o), "/"); o != "" {
			cfg.WebAuthnOrigins = append(cfg.WebAuthnOrigins, o)
		}
	}
	if cfg.DBDriver != "sqlite" && cfg.DBDriver != "postgres" {
		return Config{}, fmt.Errorf("ZAHRADA_DB_DRIVER: unsupported driver %q (sqlite|postgres)", cfg.DBDriver)
	}
	switch cfg.SMTPTLS {
	case "starttls", "tls", "none":
	default:
		return Config{}, fmt.Errorf("ZAHRADA_SMTP_TLS: unsupported mode %q (starttls|tls|none)", cfg.SMTPTLS)
	}
	defPort := "587"
	if cfg.SMTPTLS == "tls" {
		defPort = "465"
	}
	port, err := strconv.Atoi(env("ZAHRADA_SMTP_PORT", defPort))
	if err != nil || port <= 0 || port > 65535 {
		return Config{}, fmt.Errorf("ZAHRADA_SMTP_PORT: invalid port")
	}
	cfg.SMTPPort = port
	if _, err := time.LoadLocation(cfg.TimeZone); err != nil {
		return Config{}, fmt.Errorf("ZAHRADA_TIMEZONE: unknown zone %q", cfg.TimeZone)
	}
	cfg.DBLog = strings.ToLower(env("ZAHRADA_DB_LOG", "error"))
	if !slices.Contains([]string{"silent", "error", "warn", "info"}, cfg.DBLog) {
		return Config{}, fmt.Errorf("ZAHRADA_DB_LOG: use silent, error, warn or info")
	}
	if cfg.DBSlowMS, err = strconv.Atoi(env("ZAHRADA_DB_SLOW_MS", "1000")); err != nil || cfg.DBSlowMS <= 0 {
		return Config{}, fmt.Errorf("ZAHRADA_DB_SLOW_MS: invalid milliseconds")
	}
	if cfg.AllowSignup, err = envBool("ZAHRADA_ALLOW_SIGNUP"); err != nil {
		return Config{}, err
	}
	if v := os.Getenv("ZAHRADA_SECURE_COOKIES"); v != "" && v != "auto" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return Config{}, fmt.Errorf("ZAHRADA_SECURE_COOKIES: invalid value %q (auto|true|false)", v)
		}
		cfg.SecureCookies = &b
	}
	if cfg.TrustedProxies, err = httpsec.ParseProxies(os.Getenv("ZAHRADA_TRUSTED_PROXIES")); err != nil {
		return Config{}, fmt.Errorf("ZAHRADA_TRUSTED_PROXIES: %w", err)
	}
	cfg.SetupToken = strings.TrimSpace(os.Getenv("ZAHRADA_SETUP_TOKEN"))
	if cfg.DisableRateLimit, err = envBool("ZAHRADA_DISABLE_RATE_LIMIT"); err != nil {
		return Config{}, err
	}
	if cfg.DisableAPIDocs, err = envBool("ZAHRADA_DISABLE_API_DOCS"); err != nil {
		return Config{}, err
	}
	if cfg.WebhooksAllowPrivate, err = envBool("ZAHRADA_WEBHOOKS_ALLOW_PRIVATE"); err != nil {
		return Config{}, err
	}
	for _, e := range strings.Split(os.Getenv("ZAHRADA_ADMIN_EMAILS"), ",") {
		if e = strings.ToLower(strings.TrimSpace(e)); e != "" && !slices.Contains(cfg.AdminEmails, e) {
			cfg.AdminEmails = append(cfg.AdminEmails, e)
		}
	}
	if err := loadDKIM(&cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// loadDKIM reads the optional DKIM settings; errors never contain key material.
func loadDKIM(cfg *Config) error {
	cfg.DKIMDomain = strings.ToLower(strings.TrimSpace(os.Getenv("ZAHRADA_DKIM_DOMAIN")))
	cfg.DKIMSelector = strings.TrimSpace(os.Getenv("ZAHRADA_DKIM_SELECTOR"))
	pemText := os.Getenv("ZAHRADA_DKIM_PRIVATE_KEY")
	if file := os.Getenv("ZAHRADA_DKIM_PRIVATE_KEY_FILE"); file != "" {
		if pemText != "" {
			return fmt.Errorf("ZAHRADA_DKIM_PRIVATE_KEY and ZAHRADA_DKIM_PRIVATE_KEY_FILE: set only one")
		}
		b, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("ZAHRADA_DKIM_PRIVATE_KEY_FILE: %w", err)
		}
		pemText = string(b)
	}
	if cfg.DKIMDomain == "" && cfg.DKIMSelector == "" && pemText == "" {
		return nil
	}
	if cfg.DKIMDomain == "" || cfg.DKIMSelector == "" || pemText == "" {
		return fmt.Errorf("DKIM: set ZAHRADA_DKIM_DOMAIN, ZAHRADA_DKIM_SELECTOR and ZAHRADA_DKIM_PRIVATE_KEY(_FILE) together")
	}
	key, err := mail.ParseDKIMKey(pemText)
	if err != nil {
		return fmt.Errorf("ZAHRADA_DKIM_PRIVATE_KEY: %w", err)
	}
	cfg.DKIMKey = key
	return nil
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envBool(key string) (bool, error) {
	v := os.Getenv(key)
	if v == "" {
		return false, nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, fmt.Errorf("%s: invalid boolean %q", key, v)
	}
	return b, nil
}
