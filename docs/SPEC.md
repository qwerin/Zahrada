# Zahrada — specifikace (kontrakt pro implementaci)

Webová aplikace zahradnické firmy: **kalkulace realizací**, **zakázky a plán údržby zahrad**
a **docházka dodavatelů**. Self-hosted, stejný stack a provoz jako NanoFaktura, s níž se propojuje
(faktury a náklady). Navržená nejdřív pro mobil — vedoucí i dodavatelé ji používají venku na zahradě.

Tento dokument je závazný. Odchylky zapisuj do sekce *Otevřené otázky* (na konci).

## 1. Stack a struktura

Stejný jako NanoFaktura: Go 1.26 (chi + Huma v2 + GORM), SQLite (lokálně) / PostgreSQL 17 (Docker),
React 19 + TypeScript + Vite + TanStack Router/Query + Tailwind v4 + shadcn/ui, PWA. Jedna binárka
servíruje API (`/api/…`, OpenAPI na `/api/openapi.json`) i SPA. Docker image distroless, nonroot.

```
cmd/server/            server + podpříkazy (healthcheck, user …)
internal/calc/         ✅ výpočet kalkulace (čistá logika, hotovo)
internal/attendance/   ✅ pravidla docházky (čistá logika, hotovo)
internal/recurrence/   ✅ opakování údržby + české svátky (čistá logika, hotovo)
internal/config/       env ZAHRADA_*
internal/{db,model,auth,mail,storage,scheduler,secret,httpsec,ratelimit,events,ares,…}  ← F0 z NanoFaktury
internal/api/          huma handlery, jeden soubor na zdroj (+ *_test.go)
internal/nanofaktura/  klient REST API NanoFaktury (F5)
internal/pdf/          PDF nabídky (maroto, jako NanoFaktura)
web/                   SPA
```

**Pravidlo:** výpočty a pravidla žijí jen v `internal/calc`, `internal/attendance`, `internal/recurrence`.
API je volá, nikdy je nepřepisuje. Nová pravidla = nejdřív tabulkový test v daném balíčku.

## 2. Konvence

Platí konvence NanoFaktury (SPEC NanoFaktury §2) beze změny:

- **Peníze** `int64` v haléřích (API i DB), nikdy float.
- **Množství** v API decimální string (`"12.5"`, přijímá i `"12,5"`, max 3 desetinná místa), v DB `*_milli` (tisíciny).
- **Procenta** v basis points `*_bps` (`2100` = 21 %): sazba DPH, přirážka, sleva, ztratné.
- **Datum** `YYYY-MM-DD` (`*_on`), **okamžik** RFC3339 (`*_at`). ID `uint`, každý záznam má `account_id`,
  každý dotaz je jím filtrován (test tenant izolace je povinný).
- DTO vs. GORM modely, chyby problem+json s `code`, stránkování `{items, page, per_page, total}` — jako NanoFaktura.

Navíc:

- **Čas v docházce a plánu**: minuty od půlnoci místního času (`start_min`, `end_min`, `07:30` = 450),
  v API string `"HH:MM"`. Záznam nepřekračuje půlnoc. Časová zóna účtu (výchozí `Europe/Prague`).
  Binárka importuje `time/tzdata` (distroless nemá databázi zón).
- **Pracnost** v tisícinách hodiny: `norm_hours_milli` (norma na jednotku), `hours_milli` (výsledek);
  v API decimální string hodin (`"0.05"`).
- **Jednotky** (`unit`): volný text s doporučeným výběrem `m², m³, bm, ks, kg, t, hod, km, fůra, kpl`.

## 3. Uživatelé, účty a role

Celé převzato z NanoFaktury (registrace se setup tokenem, sessions, API tokeny `zh_…`, 2FA TOTP/passkeys,
pozvánky, více firem na uživatele, správa instance). Role účtu:

| Oblast | owner | admin | member (vedoucí) | accountant | contractor |
|---|---|---|---|---|---|
| Nastavení, uživatelé, integrace, uzávěrka docházky | ✔ | ✔¹ | – | – | – |
| Klienti, zahrady, dodavatelé, ceník | ✔ | ✔ | ✔ | čtení | – |
| Kalkulace (vč. nákladů a marže) | ✔ | ✔ | ✔ | čtení | – |
| Zakázky, plán, výjezdy | ✔ | ✔ | ✔ | čtení | jen `/me` |
| Docházka všech, schvalování | ✔ | ✔ | ✔ | čtení | jen `/me` |
| Přehledy, exporty | ✔ | ✔ | ✔ | ✔ | – |

¹ admin nesmí spravovat vlastníky (jako v NanoFaktuře).

**contractor** je nová role: uživatel propojený s dodavatelem (`Contractor.user_id`). Volá jen endpointy
pod `/api/accounts/{slug}/me/…` (§4.8), nikdy nevidí ceny klientům, kalkulace ani cizí docházku.
Pozvánka dodavatele se posílá z detailu dodavatele a po přijetí vyplní `user_id`.
Matice rolí se deklaruje při registraci operace (`auth.Allow`, `auth.ForEditors` …) a pokrývá ji `TestRoleMatrix`.

## 4. Doména

### 4.1 Nastavení účtu (`Account`)

Navíc k firemnímu profilu z NanoFaktury:

- `vat_payer bool`, `vat_rates []int32` (nabídka sazeb v editoru, výchozí `[2100, 1200, 0]`),
  `default_vat_rate_bps`.
- `default_hourly_rate` (haléře/h, výchozí sazba nového dodavatele), `default_km_rate`.
- `attendance_rounding_min` (0 = bez, 5, 10, 15, 30), `attendance_rounding_mode` (`nearest|up|down`),
  `attendance_locked_through` (datum, uzávěrka), `attendance_gps bool` (výchozí false).
- `plan_horizon_weeks` (výchozí 8) — jak daleko dopředu generovat výjezdy údržby.
- `quote_validity_days` (30), `quote_terms` (text podmínek do PDF), `quote_pdf_detail` (`items|sections`).
- `job_number_format` (výchozí `Z{YYYY}-{NNN}`, řada jako NumberFormat v NanoFaktuře).
- Integrace NanoFaktura (§4.10): `nf_base_url`, `nf_account_slug`, `nf_api_token` (šifrovaně, write-only).

### 4.2 Klient (`Client`)

`name`, `kind` (`person|company`), `ico`, `dic`, `email`, `phone`, `street`, `city`, `zip`, `note`,
`nf_subject_id *uint` (propojení s kontaktem v NanoFaktuře). Doplnění z ARES podle IČO (balíček `ares` z NanoFaktury).
Smazání klienta se zakázkami → 409 `has_jobs`; jinak archivace `archived bool`.

### 4.3 Zahrada (`Site`)

Místo práce; klient jich může mít víc. `client_id`, `name` („Zahrada Říčany“), `street`, `city`, `zip`,
`lat`, `lng` (volitelné, odkaz do map), `area_m2`, `lawn_m2`, `hedge_bm`, `access_note` (klíč, kód brány, pes),
`note`, fotky (přílohy `owner_type=site`). Dodavatel vidí u svých výjezdů adresu a `access_note`.

### 4.4 Dodavatel (`Contractor`)

Kdo pro firmu pracuje a eviduje docházku (OSVČ subdodavatel, brigádník, vlastní parta).
`name`, `kind` (`person|company`), `ico`, `phone`, `email`, `hourly_rate` (haléře/h, nákupní), `km_rate`,
`color` (v plánu), `active`, `user_id *uint` (přihlášení s rolí contractor), `nf_subject_id *uint`.
Změna sazby se nepromítá zpětně — záznamy docházky drží `rate_snapshot` (§4.8).

### 4.5 Ceník a sestavy

**PriceItem** — položka ceníku: `kind` (`work|material|machine|transport|subcontract|other`), `code`,
`name`, `unit`, `unit_cost` (nákupní), `unit_price *int64` (pevná prodejní) **nebo** `markup_bps`,
`waste_bps` (výchozí ztratné), `norm_hours_milli` (pracnost na jednotku), `vat_rate_bps`, `category`, `active`.
U práce lze prodejní cenu odvodit z normy a sazby: `calc.WorkUnitPrice(norm, hourly_rate)` (editor nabídne).

**Assembly** (sestava, receptura) — např. „Trávník výsevem“, jednotka m²: `name`, `unit`, `section`
a komponenty `AssemblyComponent{price_item_id, quantity_milli /* na 1 jednotku sestavy */, position}`.
Vložení sestavy do kalkulace pro N jednotek rozbalí komponenty na položky přes `calc.Expand`
(ceny se zkopírují z ceníku v okamžiku vložení — změna ceníku nemění hotové kalkulace).

Příklad sestavy „Trávník výsevem (m²)“: frézování 1 m², travní osivo 0,03 kg (ztratné 10 %),
substrát 0,05 m³, výsev a zaválcování 1 m² (norma 0,03 h).

### 4.6 Kalkulace (`Quote`, `QuoteItem`)

Kalkulace patří k zakázce a má **verze**: `job_id`, `version` (1, 2, …; unikátní v zakázce), `number`
odvozený (`Z2026-014/v2`), `title`, `status`, `valid_until`, `discount_bps` (sleva na celou kalkulaci),
`note`, uložené součty (`net`, `vat_total`, `gross`, `cost`, `margin`, `hours_milli`) přepočítané při každém uložení.

**QuoteItem**: `position`, `section`, `kind`, `price_item_id *uint`, `name`, `unit`, `quantity_milli`,
`waste_bps`, `unit_cost`, `unit_price *int64`, `markup_bps`, `discount_bps`, `vat_rate_bps`, `norm_hours_milli`, `note`.

**Výpočet** výhradně `calc.Compute`, závazné pořadí zaokrouhlení (vše na haléře, polovina od nuly):

1. množství vč. ztratného = množství × (1 + ztratné),
2. prodejní cena/j = `unit_price`, jinak `unit_cost` × (1 + přirážka),
3. řádek = množství vč. ztratného × cena/j; sleva řádku; náklad řádku = množství vč. ztratného × `unit_cost`,
4. základy po sazbách DPH (neplátce → vše sazba 0),
5. sleva kalkulace se odečte z každého základu zvlášť, pak DPH ze základu (zdola),
6. marže = základ celkem − náklady; `margin_bps` z prodejní ceny; pracnost = množství (bez ztratného) × norma.

Ověřený příklad (test `TestComputeVATPayer`): frézování 250 m² á 15 Kč, osivo 7,5 kg + 10 % ztratné á 180 Kč
+30 %, substrát 12,5 m³ á 1 290 Kč −5 % (12 %), 2 fůry á 1 200 Kč, sleva 3 % → základ 22 697,27 Kč,
DPH 3 429,10 Kč, celkem 26 126,37 Kč, marže 23,96 %, pracnost 5 h.

**Stavy**: `draft` → `sent` → `accepted` | `rejected`; `superseded` (nahrazena novější verzí).
Upravovat lze jen `draft`. `POST …/actions/new-version` zkopíruje položky do nové verze `draft`
a původní (není-li `accepted`) označí `superseded`. `accept` zamkne verzi, ostatní verze → `superseded`,
zakázka → `ordered` a `accepted_quote_id`. Nejvýš jedna `accepted` verze na zakázku.

**PDF nabídky** (`GET /quotes/{id}/pdf`): hlavička firmy, klient, zahrada, oddíly s položkami
(nebo jen součty oddílů podle `quote_pdf_detail`), rekapitulace DPH, celkem, platnost, podmínky.
Náklady, přirážky ani marže se do PDF nikdy nedostanou.

### 4.7 Zakázky a plánování

**Job** (zakázka): `number`, `client_id`, `site_id`, `kind` (`realization` = realizace, `maintenance` =
smlouva o údržbě, `one_off` = jednorázový zásah), `title`, `status`, `planned_from`, `planned_to`,
`accepted_quote_id`, `monthly_fee *int64` (paušál údržby), `note`.

Stavy: `inquiry` (poptávka) → `quoted` (odeslaná nabídka) → `ordered` (objednáno) → `in_progress` →
`done` → `invoiced`; kdykoli `cancelled`. `in_progress` nastane automaticky prvním dokončeným výjezdem
nebo docházkou na zakázku. Přechody mimo tento graf → 409 `invalid_status`.

**MaintenanceTask** (úkon údržby): `job_id` (kind `maintenance`), `site_id`, `activity` (sečení, hnojení,
stříhání živého plotu, prořez, vertikutace, podzimní úklid, zazimování závlahy …), `rule` (JSON podle
`recurrence.Rule`: `freq`, `start`, `until`, `interval_weeks`, `weekdays`, `months`, `week_of_month`,
`holidays`), `start_min` (preferovaný čas), `est_minutes`, `default_contractor_ids`, `price_per_visit *int64`, `active`.

**Visit** (výjezd — jedna návštěva zahrady v jeden den): `job_id`, `site_id`, `task_id *uint`,
`rule_date *string` (původní termín z pravidla), `date`, `start_min *int`, `est_minutes`, `status`
(`planned|done|skipped|cancelled`), `manual bool`, `note`, `done_note`, `done_at`, fotky
(`owner_type=visit`), přiřazení `VisitAssignee{visit_id, contractor_id}`.

**Generátor výjezdů** (scheduler, denně + po změně úkonu): pro každý aktivní úkon spočítá
`rule.Occurrences(dnes, dnes + plan_horizon_weeks)` a v transakci

- vloží chybějící výjezdy (unikátní index `(task_id, rule_date)`, idempotentní),
- smaže budoucí `planned` výjezdy, jejichž `rule_date` už pravidlu neodpovídá — **kromě** `manual`.

Ruční přesun nebo úprava vygenerovaného výjezdu nastaví `manual = true`; generátor ho pak nikdy nemění.
Realizace se plánují ručně výjezdy bez `task_id`.

**Týdenní plán** (`GET /plan?from=&to=`): výjezdy po dnech a dodavatelích, součet `est_minutes` na dodavatele
a den (upozornění při překročení 8 h), neobsazené výjezdy zvlášť.

### 4.8 Docházka (`AttendanceEntry`)

Jeden záznam = jeden dodavatel, jeden den, jeden úsek práce. Pravidla jsou v `internal/attendance`.

Pole: `contractor_id`, `job_id *uint` (prázdné = režie: sklad, servis strojů), `visit_id *uint`, `work_on`,
`start_min`, `end_min *int` (prázdné = otevřený záznam po „Příchodu“), `break_min`, `km`, `note`, `status`,
`rate_snapshot`, `km_rate_snapshot`, `source` (`manual|clock`), `created_by`, `approved_by`, `approved_at`,
`reject_reason`, volitelně `lat_in`, `lng_in`, `lat_out`, `lng_out` (jen při `attendance_gps`).

Pravidla:

- Validace `Entry.Validate`: platné datum, `0 ≤ start < end ≤ 24:00`, přestávka kratší než úsek, km ≥ 0 → 422.
- Překryv se stejným dodavatelem ve stejný den (`FindOverlap`, navazující úseky povoleny) → 409 `attendance_overlap`.
  Kontrola i zápis v jedné transakci.
- Uzavřené období (`Locked(work_on, attendance_locked_through)`) → 409 `period_closed` pro jakoukoli změnu.
- Sazby se při vytvoření zkopírují z dodavatele do `rate_snapshot`/`km_rate_snapshot`; vedoucí je může změnit.
- Stavy a přechody podle `attendance.Transition`: `draft → submitted → approved | rejected`,
  `rejected → submitted`, `submitted → draft` (dodavatel si vezme zpět), vedoucí smí `draft → approved`
  (zápis za dodavatele) a `approved → submitted` (znovu otevřít). Úpravy podle `attendance.Editable`.
- **Příchod/Odchod** z mobilu: `POST /me/attendance/clock-in {job_id?, visit_id?}` založí otevřený záznam
  (`start_min` = teď v zóně účtu, `source=clock`); druhý otevřený → 409 `already_clocked_in`.
  `clock-out {break_min, km, note}` doplní `end_min` a ověří pravidla. Otevřený záznam z předchozího dne
  se automaticky neuzavírá — dodavatel doplní konec ručně, vedoucí vidí upozornění.
- **Souhrn** `GET /attendance/summary?month=2026-05`: `attendance.Summarize` nad schválenými záznamy
  se zaokrouhlením účtu (každý záznam zvlášť) → po dodavatelích a zakázkách: dny, hodiny, cena práce, km, celkem.
  Neschválené záznamy v měsíci se vrací jako počet (`pending_count`), do součtů nevstupují.
- **Export** `GET /attendance/export.csv?month=` (UTF-8 s BOM, středník — pro Excel) a PDF výkaz dodavatele.
- **Uzávěrka** `POST /attendance/actions/close {through}` (owner/admin): nastaví `attendance_locked_through`;
  odmítne (409 `pending_entries`), pokud v období zbývají neschválené záznamy.

### 4.9 Přehledy

- **Rentabilita zakázky** `GET /jobs/{id}/profitability`: plán (přijatá kalkulace: základ, náklady, pracnost)
  vs. skutečnost (schválená docházka na zakázku: hodiny a cena práce + `JobCost`), rozdíl hodin a marže.
- **JobCost** (náklad zakázky mimo docházku): `job_id`, `date`, `description`, `amount`, `supplier`,
  `nf_expense_id *uint`, příloha účtenky.
- **Vytížení** `GET /reports/workload?from=&to=`: plánované minuty výjezdů vs. odpracované po dodavatelích.

### 4.10 Integrace s NanoFakturou

Přes REST API NanoFaktury (API token z *Nastavení → API tokeny*, OpenAPI na `{nf_base_url}/api/openapi.json`).
Klient v `internal/nanofaktura`, testy proti fake serveru (`httptest`), timeouty, chyby → 502 `upstream_unavailable`.

- **Kontakt**: před prvním dokladem najde kontakt podle IČO (jinak podle názvu), případně založí;
  uloží `nf_subject_id` ke klientovi/dodavateli.
- **Faktura za realizaci** `POST /jobs/{id}/actions/invoice`: vytvoří v NanoFaktuře **koncept** faktury
  z přijaté kalkulace (položky, nebo součty oddílů) se sazbami DPH; zakázka → `invoiced`, uloží se `JobInvoice{nf_invoice_id, period}`.
- **Faktura za údržbu** `POST /jobs/{id}/actions/invoice {month}`: paušál `monthly_fee` nebo dokončené výjezdy
  × `price_per_visit`; jedna faktura na zakázku a měsíc (409 `already_invoiced`).
- **Náklad dodavatele** `POST /attendance/summary/{contractor_id}/actions/expense {month}`: volitelně založí
  přijatý doklad (náklad) v NanoFaktuře podle souhrnu; obvykle ale dodavatel pošle vlastní fakturu.
- Vystavení (číslo dokladu) zůstává v NanoFaktuře — Zahrada vytváří jen koncepty.

## 5. Konfigurace (env)

Jako NanoFaktura, prefix `ZAHRADA_` místo `NANOFAKTURA_` (stejné názvy a výchozí hodnoty):
`PUBLIC_URL`, `LISTEN_ADDR`, `DB_DRIVER`, `DB_DSN`, `STATIC_DIR`, `DATA_DIR`, `SETUP_TOKEN`, `SECRET_KEY`,
`ALLOW_SIGNUP`, `SECURE_COOKIES`, `TRUSTED_PROXIES`, `DISABLE_RATE_LIMIT`, `DISABLE_API_DOCS`, `ADMIN_EMAILS`,
`SMTP_*`, `MAIL_FROM`, `DKIM_*`, `DB_LOG`, `DB_SLOW_MS`. Navíc `ZAHRADA_TIMEZONE` (výchozí `Europe/Prague`).

## 6. Frontend

Mobilní první, PWA, čeština. Shell, přihlášení, nastavení a komponenty převzít z NanoFaktury.

- **Dnes** (vedoucí): dnešní výjezdy po dodavatelích, kdo je „v práci“ (otevřené záznamy), čekající docházka.
- **Plán**: týden, řádky = dodavatelé, sloupce = dny; přetažení výjezdu mění den/dodavatele (`manual=true`).
- **Zakázky**: seznam s filtrem stavu a druhu; detail se záložkami Přehled, Kalkulace, Výjezdy, Docházka, Rentabilita.
- **Editor kalkulace**: oddíly a položky, vložení z ceníku a sestav, živý přepočet přes `POST /calc/preview`
  (bez uložení), přepínač verzí, náhled PDF. Náklad a marže viditelné jen v aplikaci.
- **Klienti a zahrady, Dodavatelé, Ceník a sestavy**.
- **Docházka** (vedoucí): měsíc × dodavatelé, hromadné schválení, uzávěrka, export.
- **Moje práce** (dodavatel, jen `/me`): dnešní výjezdy s adresou, poznámkou k přístupu a odkazem do map,
  velké tlačítko Příchod / Odchod, moje docházka za měsíc a odeslání ke schválení.

## 7. Nasazení a CI/CD

- GitHub Actions (`.github/workflows/ci.yml`): Go testy + govulncheck, typecheck a build frontendu,
  build image. Push do `main` → `ghcr.io/qwerin/zahrada:main` a `:sha-xxxxxxx`; tag `v1.2.3` →
  `:1.2.3`, `:1.2`, `:latest`. Poté webhook Portaineru (secret `PORTAINER_WEBHOOK_DEV` / `_PROD`).
- Portainer: dva stacky z Git repozitáře (`docker-compose.yml` + `docker-compose.traefik.yml`):
  **dev** (`APP_TAG=main`, `DOMAIN=zahrada-dev…`, `ROUTER=zahrada-dev`) a **prod** (`APP_TAG=latest`
  nebo pevná verze). Vlastní volumes a databáze pro každý stack.
- Release: `git tag v0.3.0 && git push --tags`. Rollback: v prod stacku nastavit `APP_TAG` na předchozí verzi.
- Migrace: GORM AutoMigrate při startu (jako NanoFaktura); destruktivní změny jen ručním SQL v `docs/migrations/`.
- Zálohy: `pg_dump` databáze + volume `/data` (fotky, `secret.key`).

## 8. Fáze implementace

Každá fáze končí zeleným CI a nasazením na dev stack.

- **B — zaváděcí (hotovo)**: doménová jádra `calc`, `attendance`, `recurrence` s testy; zaváděcí server
  (`/api/health`, `/api/calc/preview`, SPA, `healthcheck`); Dockerfile, compose, Traefik overlay, CI → GHCR → Portainer.
  *Hotovo, když* dev stack na HTTPS ukazuje verzi z CI a náhled kalkulace.
- **F0 — platforma z NanoFaktury**: převzít `config`, `db`, `model` (User, Account, Membership, Session,
  ApiToken, Invitation, Attachment, Event, Todo), `auth`, `mail`, `maildiag`, `storage`, `scheduler`, `secret`,
  `httpsec`, `ratelimit`, `events`, `search`, `slug`, `ares`, testovací harness `internal/api/testutil_test.go`,
  `cmd/server` (bez fakturačních podpříkazů) a web shell (router, layout, přihlášení, 2FA, nastavení, členové).
  Odstranit vše fakturační (doklady, banka, DPH, ISDOC, ČNB, VIES, pravidelné faktury, MCP).
  Přejmenovat `NANOFAKTURA_*` → `ZAHRADA_*`, token prefix `nf_` → `zh_`, cookie, texty. Přidat roli `contractor`.
  Zaváděcí `cmd/server/api.go` nahradit huma operací `POST /api/accounts/{slug}/calc/preview`.
  *Hotovo, když* projdou převzaté testy auth/účtů/rolí a tenant izolace.
- **F1 — evidence**: klienti (ARES), zahrady, dodavatelé (+ pozvánka s rolí contractor), ceník, sestavy.
- **F2 — kalkulace**: zakázka (základ), kalkulace s verzemi, editor, `expand` sestav, PDF, odeslání e-mailem, přijetí.
- **F3 — plánování**: zakázky (celý životní cyklus), úkony údržby, generátor výjezdů, týdenní plán, výjezdy pro dodavatele.
- **F4 — docházka**: CRUD, Příchod/Odchod, schvalování, uzávěrka, souhrn, CSV/PDF export, obrazovka Moje práce.
- **F5 — peníze**: JobCost, rentabilita, vytížení, integrace s NanoFakturou (kontakty, koncepty faktur, náklady).

Mimo rozsah (zatím): optimalizace tras, sklad materiálu, klientský portál, offline zápis docházky
se synchronizací, předpověď počasí v plánu.

## Otevřené otázky

1. **Kdo je „dodavatel“** — jen OSVČ subdodavatelé, nebo i vlastní lidé na DPP/HPP? Model to unese oboje,
   ale u zaměstnanců by docházka měla pracovněprávní náležitosti (mimo rozsah této verze).
2. **Fakturace údržby** — paušál měsíčně, nebo za každý výjezd? Model podporuje obojí (§4.10).
3. **Sazby DPH** u realizací volí uživatel u položky; aplikace je neodvozuje. Výchozí sazby ověřit s účetní.
4. **GPS u Příchodu/Odchodu** — ve výchozím stavu vypnuto; před zapnutím informovat dodavatele (GDPR).
5. **Parta na jednom záznamu** — eviduje se vždy po osobách; hromadný zápis „celá parta 7:00–15:30“
   může UI rozepsat na víc záznamů (stejná pravidla překryvů).
