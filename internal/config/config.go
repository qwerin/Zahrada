// Package config čte nastavení z proměnných prostředí ZAHRADA_* (SPEC §5).
// Ve fázi F0 se rozšíří o proměnné převzaté z NanoFaktury (DB, SMTP, zabezpečení …).
package config

import (
	"os"
	"strings"
)

type Config struct {
	ListenAddr string // ZAHRADA_LISTEN_ADDR, výchozí :8080
	StaticDir  string // ZAHRADA_STATIC_DIR, build frontendu (v Dockeru /app/dist)
	PublicURL  string // ZAHRADA_PUBLIC_URL
	TimeZone   string // ZAHRADA_TIMEZONE, výchozí Europe/Prague (docházka, plán)
}

func env(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

// Load načte konfiguraci z prostředí.
func Load() Config {
	return Config{
		ListenAddr: env("ZAHRADA_LISTEN_ADDR", ":8080"),
		StaticDir:  env("ZAHRADA_STATIC_DIR", ""),
		PublicURL:  env("ZAHRADA_PUBLIC_URL", "http://localhost:8080"),
		TimeZone:   env("ZAHRADA_TIMEZONE", "Europe/Prague"),
	}
}
