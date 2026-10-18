# CLAUDE.md

Zahrada — self-hosted app for a garden-services company: quote calculations, jobs and maintenance
planning, contractor attendance. Same stack and conventions as NanoFaktura (github.com/qwerin/nanofaktura),
which it integrates with for invoices and expenses.

**`docs/SPEC.md` is the binding contract** (data model, rules, API, phases). Read it before changing
anything; record any deviation in its "Otevřené otázky" section. User-facing texts are Czech.

## Current state

- Platform layer ported from NanoFaktura (auth, 2FA, accounts, members, invitations, attachments,
  events, todos, search, mail, scheduler) without invoicing. New role `contractor` (SPEC §3): only
  operations declaring `auth.ForEveryone` (account detail, `/me/*`); undeclared = staff only.
- Done (SPEC §4.2–4.8): clients + sites, contractors (+ invitation linking a login), jobs (numbering,
  status graph, assignment), attendance (staff CRUD, approvals, close, summary, CSV; contractor
  clock-in/out and own entries), calc preview. Web: dashboard, jobs, clients, contractors, attendance,
  „Moje práce“, settings.
- Price list (`/price-items`, bulk import) and quotes per job with versions (`/jobs/{id}/quotes`,
  `/quotes/{id}`): draft → sent → accepted (job → ordered, other versions superseded). The offer is
  printed from `/tisk/{slug}/nabidka/{id}` (browser print → PDF, no costs/margins). Paste import of
  items (chat tables, Excel, free lines) is `web/src/lib/paste-items.ts` (+ tests).
- Job photos: attachments with owner_type `job`; contractors upload/view via `/me/jobs/{id}/photos`
  and `/me/photos/{id}` (only assigned jobs, images only).
- MCP server at `/api/mcp` (`internal/api/mcp.go`): curated tools dispatched in-process to the REST API with the
  caller's token (same roles/validation); OAuth for claude.ai connectors in `oauth.go`. New tool = entry in
  `mcpTools` + a call in `TestMCPEveryTool`; never expose deletions, approvals, sending or settings.
- Contractors without e-mail: `User.LinkOnly` (placeholder `*@bez-emailu.invalid`, no password) signs in only
  with one-time `LoginLink`s (`POST /contractors/{id}/login-link`, `POST /api/auth/login-link`, SPA `/prihlaseni/{token}`).
  A new link deletes the user's sessions; deleting the contractor deletes the link-only user.
- Not yet: assemblies, maintenance tasks + visit generator and weekly plan, job costs/profitability,
  NanoFaktura integration (SPEC §8 F3, F5).
- Tests: `internal/api/garden_test.go` covers the garden domain, roles and tenant isolation.
  SQLite runs single-connection: inside a transaction use only `tx` (helpers take a `db` param).

## Commands

```bash
make test           # go vet + go test ./... + web typecheck (if web/node_modules exists)
make run            # build web + binary, serve on :8080
make dev-backend    # go run ./cmd/server/  (:8080)
make dev-frontend   # Vite on :5173, proxies /api → :8080
make docker-dev     # full stack locally (Postgres + app built from source)
go test ./internal/calc/ -run TestCompute -v -count=1
```

## Domain rules live in pure packages

`internal/calc`, `internal/attendance`, `internal/recurrence` have no DB/HTTP imports and are the only
place where money, attendance and scheduling rules are implemented. API handlers convert DTO ↔ package
types and call them; never re-implement rounding, overlap, status transitions or recurrence in handlers
or in the frontend (the editor previews via `POST …/calc/preview`). A new rule = a table test in the
package first, then the handler.

- `calc.Compute(items, opts)` — rounding order is binding (SPEC §4.6). `*calc.ItemError{Index, Field}`
  maps to 422 `items[i].field`. `calc.ParseMilli`/`FormatMilli` for decimal strings in the API.
- `attendance`: `Entry.Validate`, `FindOverlap` (run inside the write transaction), `Locked`, `Editable`,
  `Transition(from, to, manager)`, `Summarize(approvedEntries, rounding)`. Times are minutes from local
  midnight; `MinuteOfDay(now, accountTZ)` for clock-in/out.
- `recurrence.Rule.Occurrences(from, to)` — generator inserts missing `(task_id, rule_date)` visits and
  never touches `manual` ones (SPEC §4.7).

## Conventions

Money `int64` haléře; quantities decimal string in API / `*_milli` in DB; percentages `*_bps`; dates
`YYYY-MM-DD`; every record has `account_id` and every query is scoped (tenant-isolation test is mandatory).
Errors problem+json with machine-readable `code`. Details: SPEC §2 and NanoFaktura's CLAUDE.md.

## Novinky (CHANGELOG.md)

`CHANGELOG.md` in the repo root is shown in the app (*Novinky*, `web/src/hooks/use-news.ts`, parser
`web/src/lib/changelog.ts`) and is copied into the Docker frontend stage. Every user-visible change gets
an entry **in the same commit**: newest first, `## YYYY-MM-DD · Krátký název` (one entry per date —
extend today's entry instead of adding a second one), bullets `- **Co** — k čemu je to dobré.`, Czech,
written for users (no technical jargon). Internal changes don't go there.

## Deploy

CI (`.github/workflows/ci.yml`) pushes `ghcr.io/qwerin/zahrada:main` on every push to main and
`:X.Y.Z` + `:latest` on `v*` tags, then calls the Portainer stack webhook. Do not add `build:` to
`docker-compose.yml` — Portainer pulls images; local builds use `docker-compose.dev.yml`.
