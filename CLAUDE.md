# CLAUDE.md

Zahrada — self-hosted app for a garden-services company: quote calculations, jobs and maintenance
planning, contractor attendance. Same stack and conventions as NanoFaktura (github.com/qwerin/nanofaktura),
which it integrates with for invoices and expenses.

**`docs/SPEC.md` is the binding contract** (data model, rules, API, phases). Read it before changing
anything; record any deviation in its "Otevřené otázky" section. User-facing texts are Czech.

## Current state: phase B (bootstrap)

- Done: pure domain packages with table tests — `internal/calc` (quote engine), `internal/attendance`
  (attendance rules), `internal/recurrence` (maintenance schedules + Czech holidays).
- Bootstrap server `cmd/server` on stdlib `net/http` (`/api/health`, `/api/calc/preview`, SPA, `healthcheck`
  subcommand). No dependencies yet, so there is no `go.sum`.
- Bootstrap SPA in `web/` (Vite + React, plain CSS) — a calculation preview page.
- Next: **phase F0** (SPEC §8) — port the platform layer from NanoFaktura. Clone it next to this repo and
  copy packages, then rename `NANOFAKTURA_*` → `ZAHRADA_*`, `nf_` tokens → `zh_`, remove invoicing code,
  add the `contractor` role. Replace `cmd/server/api.go` with huma operations; keep `healthcheck`
  and the `time/tzdata` import. Port NanoFaktura's CLAUDE.md sections (adding endpoints, roles, errors,
  tests harness) here once the code is in.

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

## Deploy

CI (`.github/workflows/ci.yml`) pushes `ghcr.io/qwerin/zahrada:main` on every push to main and
`:X.Y.Z` + `:latest` on `v*` tags, then calls the Portainer stack webhook. Do not add `build:` to
`docker-compose.yml` — Portainer pulls images; local builds use `docker-compose.dev.yml`.
