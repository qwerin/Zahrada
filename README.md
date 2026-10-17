# Zahrada

Kalkulace zahradních realizací, plánování zakázek a údržby zahrad a docházka dodavatelů.
Self-hosted, stejný stack jako [NanoFaktura](https://github.com/qwerin/nanofaktura) a propojení s ní
(koncepty faktur, náklady).

Stav: **fáze B (zaváděcí)** — hotová a otestovaná výpočetní jádra (kalkulace, docházka, opakování údržby),
zaváděcí server s náhledem kalkulace a kompletní řetězec CI → GHCR → Portainer → Traefik.
Plán dalších fází a závazná specifikace: [`docs/SPEC.md`](docs/SPEC.md), pravidla pro vývoj: [`CLAUDE.md`](CLAUDE.md).

## Lokálně

```bash
# Go 1.26+ a Node 22+
make test          # Go testy + typecheck
make run           # → http://localhost:8080
# nebo vývoj s hot reloadem:
make dev-backend   # :8080
make dev-frontend  # :5173
```

Celý stack v Dockeru (Postgres + aplikace z vlastního buildu):

```bash
cp .env.example .env    # vyplň POSTGRES_PASSWORD
make docker-dev         # → http://localhost:8080
```

## CI/CD

GitHub Actions při každém pushi spustí testy a sestaví image:

| Událost | Image v GHCR | Nasazení |
|---|---|---|
| push do `main` | `ghcr.io/qwerin/zahrada:main`, `:sha-xxxxxxx` | webhook dev stacku |
| tag `v1.2.3` | `:1.2.3`, `:1.2`, `:latest` | webhook produkce |
| pull request | jen build, bez push | — |

Vydání verze: `git tag v0.1.0 && git push --tags`.

Repozitář → *Settings → Secrets and variables → Actions*:

- `PORTAINER_WEBHOOK_DEV`, `PORTAINER_WEBHOOK_PROD` — URL webhooků stacků (volitelné; bez nich se krok přeskočí).
  Portainer musí být z GitHubu dostupný přes HTTPS. Pokud není, nech webhooky prázdné a použij
  v Portaineru *GitOps updates → Polling* nebo ruční *Pull and redeploy*.

## Produkce — Portainer + Traefik

Předpoklad: Traefik běží v externí síti (např. `proxy`) s entrypointy `web`/`websecure` a cert resolverem.

1. **Přístup k image.** Package `zahrada` v GHCR je ve výchozím stavu soukromý: buď ho přepni na public
   (GitHub → Packages → zahrada → Package settings), nebo v Portaineru přidej *Registries → Custom registry*
   `ghcr.io` s uživatelem GitHubu a tokenem s právem `read:packages`.
2. **Stack** → *Add stack* → *Repository*:

   | Pole | Hodnota |
   |---|---|
   | Repository URL | `https://github.com/qwerin/zahrada` |
   | Repository reference | `refs/heads/main` |
   | Compose path | `docker-compose.yml` |
   | Additional paths | `docker-compose.traefik.yml` |

3. **Environment variables** (vzor v `.env.example`):

   | Proměnná | dev stack | produkce |
   |---|---|---|
   | `APP_TAG` | `main` | `latest` (nebo pevná verze) |
   | `DOMAIN` | `zahrada-dev.example.cz` | `zahrada.example.cz` |
   | `ROUTER` | `zahrada-dev` | `zahrada` |
   | `CERT_RESOLVER` | `letsencrypt` | `letsencrypt` |
   | `TRAEFIK_NETWORK` | `proxy` | `proxy` |
   | `POSTGRES_PASSWORD` | silné heslo | jiné silné heslo |

4. **GitOps updates** zapnout → *Mechanism: Webhook*, zapnout *Re-pull image* a *Force redeployment*.
   Zkopírovanou URL webhooku ulož do secretu `PORTAINER_WEBHOOK_DEV` / `_PROD`.
5. Ověření: `https://zahrada-dev.example.cz/api/health` vrátí `{"status":"ok","version":"main+abc1234"}`.

Overlay nastaví `ZAHRADA_PUBLIC_URL=https://${DOMAIN}`, Secure cookie, důvěru v `X-Forwarded-*` od Traefiku,
`traefik.docker.network` (kontejner je ve dvou sítích) a zruší publikovaný port 8080 (vyžaduje Docker Compose ≥ 2.24).
Rollback produkce: v proměnných stacku nastav `APP_TAG` na předchozí verzi a *Update the stack*.

## Struktura

```
cmd/server/            server (zaváděcí; ve fázi F0 huma API z NanoFaktury)
internal/calc/         výpočet kalkulace — ztratné, přirážky, slevy, DPH po sazbách, marže, pracnost, sestavy
internal/attendance/   docházka — validace, překryvy, zaokrouhlení, schvalování, uzávěrka, měsíční souhrn
internal/recurrence/   opakování údržby (týdně/měsíčně, sezóna) a české svátky
web/                   SPA (React + Vite)
docs/SPEC.md           závazná specifikace a plán fází
```
