# Pricing platform v0.1

A runnable PostgreSQL → sqlc/pgx → Go REST data service and Python → REST quote ingestion slice, with a C++ European Black-Scholes library connected to Go REST through cgo. All demonstration prices are synthetic.

**Frontend:** Angular 22 with NgRx Signal Store. Components invoke store methods and consume signals; stores own state and trigger injected HTTP services. The option workspace includes search, instrument selection, quotes, expiration selection, paired call/put rows, refresh, loading, error, and empty states. A European option pricer lets you submit explicit sample or custom scenarios to the C++ engine, with a yield-curve browser for explicit zero-rate selection. See docs/frontend-design.md.

## Local run

Prerequisites: Docker with Compose, Go 1.24+, PostgreSQL `psql`, Python 3.11+, CMake 3.16+, and C/C++17 compilers. The Go API and Go tests now require `CGO_ENABLED=1` and the C++ runtime (Linux: GCC/G++ and libstdc++; macOS: Xcode command-line tools). Native integration is verified on Linux. Go compiles the C++ sources directly; no separate CMake build is needed to run the API. Cross-compilation requires a matching C/C++ cross-toolchain. Generated Go database code is committed; install sqlc v1.31.1 only when editing queries (`go install github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1`).

From this directory:

```sh
make db-up
make migrate
make seed
USERADMIN_PASSWORD='choose-a-long-password' go run ./cmd/useradmin user -email you@example.com -role admin
make api
```

In another terminal in this directory:

```sh
export PRICING_API_KEY=$(go run ./cmd/useradmin api-key -email you@example.com -name local-ingest -scope ingest)
make sample
make smoke
make test
```

`make sample` submits a synthetic quote with the current observation time. `make smoke` verifies the security master, price bars, bond analytics, performance statistics, resolution, search, option expirations/chain, curves, ingestion, decimal precision, idempotency, late arrival ordering, pricing through the C++ engine, and validation over HTTP against real PostgreSQL. It adds synthetic quotes to the local sample database, and provisions its own local admin (`smoke-admin@example.test`) and an ingest key on each run; it refuses a `DATABASE_URL` that is not on `127.0.0.1` or `localhost`.

API: `http://127.0.0.1:8080`. Postgres: `127.0.0.1:54329`, database/user/password `pricing`. These are local development defaults. Override `DATABASE_URL` and `HTTP_ADDR` as needed. Every route except `/healthz` and the login, refresh and sign-up endpoints requires a bearer token; see **Authentication**. Stop API with Ctrl-C and database with `docker compose stop` (data is retained).

```sh
curl 'http://127.0.0.1:8080/v1/instruments/resolve?scheme=ticker&value=AAPL&venue=XNAS'
curl 'http://127.0.0.1:8080/v1/options/00000000-0000-0000-0000-000000000001/chain?expiration=2027-01-15'
```

## Authentication

All routes except `/healthz`, login, refresh and sign-up require `Authorization: Bearer <token>`. One scheme serves the web app and the iOS/Android apps: there are no cookies, so no CSRF surface, and native clients store tokens in the Keychain or Keystore.

| Caller | Read routes (GETs, `/v1/pricing/european`, `/v1/analytics/bond`) | Write routes (`/v1/ingestion/*`, `POST /v1/instruments/*`) | Admin routes (`/v1/auth/api-keys`) |
| --- | --- | --- | --- |
| `user` session | yes | 403 | 403 |
| `admin` session | yes | yes | yes |
| API key, scope `read` | yes | 403 | 403 |
| API key, scope `ingest` | yes | yes | 403 |

- **Sessions.** Login returns an access token (`ipa_…`, 15 minutes) and a refresh token (`ipr_…`, 30 days); override with `ACCESS_TOKEN_TTL` and `REFRESH_TOKEN_TTL` (Go durations, e.g. `30m`, `720h`). Each refresh returns a new pair and retires the old one. Presenting a retired refresh token again is treated as theft and revokes every token from that login, so **clients must serialize refreshes** (one in flight, shared by concurrent requests). Logout takes the refresh token (so it works after the access token has expired) and revokes that login only; other devices stay signed in.
- **API keys** (`ipk_…`) are for machines such as the Python ingestion pipeline. An admin creates them over the API or with `cmd/useradmin`; the key is shown once. `RestSink` and `pricing-ingest` read `PRICING_API_KEY` (or `--api-key`).
- **Storage.** Passwords use argon2id (19 MiB, 2 passes; parameters are stored per hash). Tokens are 256-bit random values; only their SHA-256 digests are stored, so a database leak exposes no usable credential. Each authenticated request does one indexed lookup.
- **Accounts.** Self-service sign-up is off unless `AUTH_ALLOW_SIGNUP=true` and creates `user`-role accounts. Admins are created with `go run ./cmd/useradmin user -email … -role admin` (password from `USERADMIN_PASSWORD` or stdin; re-running it updates the password and role and re-enables the account). Passwords must be 12–128 characters.
- **Abuse limits.** After 10 failed logins for one email in 15 minutes, that email gets 429 until the window ends (this also means an attacker can temporarily lock an account). Sign-up is limited to 5 per client address per hour. Limits are in memory and per instance. Unknown emails cost the same password work as wrong passwords.
- **Local development only.** `AUTH_DISABLED=true` (`make api-noauth`) opens every route for API exploration and is refused unless `HTTP_ADDR` is a loopback address. The web app always shows its sign-in page, so use `make api` with it.

**Web app.** `/login` signs in and returns to the page the user asked for (same-origin paths only). An HTTP interceptor adds the access token to `/v1` calls and refreshes it shortly before it expires. On a 401 it makes one shared refresh and retries once; if the refresh is rejected, the user returns to sign-in with a "session expired" notice. The access token is kept in memory only. The refresh token is stored in `sessionStorage` (this tab, cleared when it closes), or in `localStorage` when **Keep me signed in** is checked. Tabs that share `localStorage` serialize refreshes with the Web Locks API and re-read the stored token inside the lock, so two tabs never spend the same single-use refresh token. Signing out in one tab signs out the others. Trade-off: a persisted refresh token is readable by script running on the page (XSS). The app loads no third-party scripts; an HttpOnly refresh cookie is the stronger option if that changes.

Not yet implemented: password change and reset, email verification, MFA, disabling users through the API, OAuth/social sign-in, and pruning expired session rows.

## Neon (hosted PostgreSQL)

The API and migrations run unchanged against [Neon](https://neon.tech); only the connection strings differ. In the Neon console create a project and a `pricing` database, then copy both connection strings from **Connect**: the **pooled** one (host contains `-pooler`) for the API, and the **direct** one for migrations, which run DDL inside a single transaction under an advisory lock. Keep `sslmode=require`.

```sh
cp .env.example .env        # fill in the two Neon URLs; .env is gitignored
set -a; . ./.env; set +a
make migrate                # uses MIGRATE_DATABASE_URL, falling back to DATABASE_URL
make seed                   # optional synthetic sample data
make api
```

Neon suspends idle computes; the API allows 20 seconds for the first ping (a cold start takes a few seconds) and recycles idle pool connections after four minutes. When `HTTP_ADDR` is unset, the API binds `:$PORT` if `PORT` is set (Cloud Run and similar), else `127.0.0.1:8080`. Never commit credentials; use your platform's secret store for `DATABASE_URL` in deployed environments. Neon branches are a convenient way to give each feature branch or CI run its own copy of the database.

To install Python as a package, optionally create a virtual environment and run `pip install -e ./ingestion`; then use `pricing-ingest`. The Make targets need no Python dependencies.

## Market data ingestion

`pricing-ingest` (or `python -m pricing_ingestion.cli`) normalizes vendor data in Python and posts it to `/v1/ingestion/*` with an ingest-scoped `PRICING_API_KEY`. It uses the standard library only.

```sh
export PRICING_API_KEY=ipk_…            # ingest scope, from cmd/useradmin or POST /v1/auth/api-keys
export MASSIVE_API_KEY=…                 # from massive.com
make ingest-massive TICKERS=AAPL,MSFT FROM=2025-11-01 TO=2026-10-30
make ingest-treasury YEAR=2026           # or LATEST=1 for the most recent date only
```

**Massive (formerly Polygon.io): daily stock bars.**
- For each ticker the adapter fetches the ticker overview. It resolves an existing instrument by composite FIGI, then by `ticker@primary exchange`.
- If neither matches, it creates the equity with ticker, FIGI, share-class FIGI and CIK identifiers.
- It then loads raw daily OHLCV, plus the split-adjusted close as `adj_close` (Massive's adjusted bars are split-adjusted only, not dividend-adjusted). Bars go to the API in batches of up to 5,000.
- The Massive key travels in an `Authorization` header, never in URLs.
- Requests are paced to the free tier's 5 per minute. Set `MASSIVE_CALLS_PER_MINUTE` (or `--calls-per-minute`) higher on paid plans.
- HTTP 429 and 5xx responses are retried with backoff, honouring `Retry-After`.
- `MASSIVE_BASE_URL` defaults to `https://api.massive.com`; `https://api.polygon.io` also works.
- Re-running is safe: replayed bars are counted as not new.

**U.S. Treasury: zero curve.**
- The adapter downloads the year's Daily Treasury Par Yield Curve CSV, or reads `--file`. It finds tenors from the column headers, so added columns such as 1.5-month need no code change, and it skips blank cells.
- Each date's par curve is bootstrapped into continuously compounded zero rates and stored as one snapshot of the curve **"UST zero (bootstrapped from par CMT)"** (USD, source `treasury`), observed at 15:30 New York time.
- Bootstrap method: par yields are interpolated linearly. Maturities of six months or less are treated as bills, with discount factor 1/(1 + y·t). Longer ones are semiannual par bonds on a 0.5-year grid, solved one step at a time.
- Each snapshot has the published sub-six-month tenors plus 60 semiannual points out to 30 years, with tenor days = round(years × 365).
- Use its `curve_id` with `GET /v1/bonds/{id}/analytics` for a Z-spread to Treasuries.
- The result approximates the published fitted par curve: actual coupon dates and bill/coupon basis differences are ignored.

`POST /v1/ingestion/curves` takes `{"curve_name","currency","source_id","observed_at","points":[{"tenor_days","zero_rate"}]}`. It creates the curve on first use and returns `{"curve_id","inserted"}`; a replayed curve, source and observation time keeps its first points. Migration 000004 registers the `massive` and `treasury` data sources in every database (Neon included). Vendor terms: the cheaper Massive plans are licensed for personal use; showing their data to other users needs a business licence.

## Run the frontend

Use Node.js 24 LTS or another Angular 22 supported release and npm. With the API running (`make api`) and a user created with `cmd/useradmin` (see **Local run**), in a third terminal:

```sh
cd web
npm ci
npm start
```

Open http://127.0.0.1:4200, sign in, and select **Apple Inc. (sample)**. Signing in is described under **Authentication → Web app**. Development requests to `/v1` are proxied to the local Go API. For production hosting, route `/v1` to Go on the same origin; the development proxy is not included in the build. `npm run build` writes to `web/dist/web`.

The **European option pricer** below the market workspace starts with the documented sample scenario. Edit the call/put choice, spot, strike, year fraction, volatility, rate, and dividend yield, then choose **Calculate price**. The sample scenario is independent of instrument selection. To start from a contract, choose **Use call** or **Use put** in the option chain; this copies strike/right and clears market assumptions. Explicitly copy an underlying bid or ask, or enter spot manually. Enter a valuation date and choose **Use ACT/365F** to calculate calendar days to expiration divided by 365 (UTC date-only, rounded to eight decimals), or enter years manually. Volatility and rates must be supplied. Editing inputs clears old results and cancels pending calculations. Changing instrument, expiry, or refreshing clears the linked contract. **Reset contract inputs** restores the contract with empty assumptions; **Use sample scenario** returns to the sample defaults.

Open **Yield curves · choose a scenario rate** inside the pricer, load a three-letter currency, and select a curve to view its latest snapshot. Linked contracts lock the currency to the underlying currency. **Use rate** copies the selected point exactly into the risk-free rate field, retaining its curve, tenor, source, and observation time. The selected tenor is used as a flat scenario assumption; there is no interpolation, automatic tenor matching, or historical as-of lookup. Refreshing or browsing curves does not change an already copied rate. Rates outside the pricing API's decimal precision remain visible but cannot be copied; enter any rounded assumption manually.

Quote values and model inputs/results remain decimal strings in state and rendering; model results still reflect C++ floating-point arithmetic. Strike ordering converts to a number for display sorting only. Google Fonts are optional presentation assets with system fallbacks. No vendor API calls or credentials are used in the browser.

## Boundaries

```text
cmd/api → transport/rest → service → domain.Store ← repository → internal/db → PostgreSQL
Python providers → normalized Quote → REST
Go REST → service.Pricer → domain.PricingEngine ← cgo/C ABI → C++ Black-Scholes
C++ EuropeanOption + MarketSnapshot → PriceResult
web/ → components → NgRx Signal Stores → HTTP services → Go REST
```

- `internal/domain`: persistence-independent models and store interface.
- `internal/service`: validation and use cases; no pgx/sqlc types.
- `internal/repository`: explicit conversion between domain and sqlc models.
- `internal/db`: generated code; edit `db/queries`, then `make generate`.
- `cmd/migrate`: embedded, ordered, transactional migrations with checksums and an advisory lock. Re-running is safe; applied migration edits are rejected. Down SQL is provided for disposable database recovery but is intentionally not automatically invoked.
- `ingestion`: provider/sink protocols, normalized decimal quote model and REST client. Massive (daily bars, reference data) and Treasury (zero curves) adapters are implemented; FINRA and FRED adapters explicitly raise `NotImplementedError`. No vendor payloads or credentials cross into Go/C++; Python never accesses PostgreSQL.
- `pricing`: a C++ library with no persistence, network or vendor dependency. The Go adapter in `pricing/src` implements `domain.PricingEngine`; a C ABI catches all native exceptions before returning to Go. The adapter lives alongside the canonical C++ source so Go builds it directly and tracks source changes. `cmd/api` injects it into `service.Pricer`. CMake still builds and tests the standalone C++ library.

## REST contract

| Method | Path | Result |
| --- | --- | --- |
| GET | `/healthz` | Process liveness (public) |
| POST | `/v1/auth/login` | `{"email","password"}` → access and refresh tokens (public) |
| POST | `/v1/auth/refresh` | `{"refresh_token"}` → a new token pair; the old pair stops working (public) |
| POST | `/v1/auth/register` | Self-service `user` account, only when `AUTH_ALLOW_SIGNUP=true` (public) |
| POST | `/v1/auth/logout` | `{"refresh_token"}` → revoke that login's tokens; always 200 (public) |
| GET | `/v1/auth/me` | The authenticated caller |
| POST/GET | `/v1/auth/api-keys` | Admin session: create (key returned once) / list API keys |
| DELETE | `/v1/auth/api-keys/{id}` | Admin session: revoke an API key |
| GET | `/v1/instruments?q=AAPL` | Up to 50 current active instruments; deterministic name/ID order |
| GET | `/v1/instruments/resolve?scheme=ticker&value=AAPL&venue=XNAS` | Exact, case-sensitive identifier matches; array preserves ambiguity |
| GET | `/v1/market-data/{id}/quote` | Latest observed quote, 404 if absent |
| GET | `/v1/options/{underlyingId}/expirations` | Sorted date strings; includes historical expirations |
| GET | `/v1/options/{underlyingId}/chain?expiration=2027-01-15` | Contracts sorted by strike/right with latest available quote |
| GET | `/v1/curves?currency=USD` | Curve definitions |
| GET | `/v1/curves/{id}/points` | Latest snapshot points, sorted by tenor days |
| POST | `/v1/pricing/european` | Stateless European Black-Scholes scenario price, per unit |
| POST | `/v1/ingestion/quotes` | Persist normalized quote; 200 accepted, including identical replay |
| POST | `/v1/instruments/equities` | Create an equity with identifiers; 201 `{"id"}`, 409 on a duplicate identifier |
| POST | `/v1/instruments/bonds` | Create a bond with terms, call schedule and identifiers; 201 `{"id"}`, 409 on duplicate |
| GET | `/v1/bonds/{id}` | Bond terms, current identifiers and call schedule |
| GET | `/v1/bonds/{id}/analytics?settlement=2026-09-15&source=sample` | Analytics from the latest bar close on or before settlement; or pass `clean_price` or `yield`; optional `curve_id` adds a Z-spread |
| POST | `/v1/analytics/bond` | Stateless bond analytics scenario |
| GET | `/v1/market-data/{id}/bars?source=sample&from=2026-01-01&to=2026-09-15` | Daily bars for one source, ascending, up to 20,000 |
| GET | `/v1/instruments/{id}/performance?source=sample&from=…&to=…` | Return/risk statistics; optional `benchmark_id`, `risk_free_rate` |
| POST | `/v1/ingestion/bars` | Persist up to 5,000 daily bars in one transaction; `{"received","inserted"}` |
| POST | `/v1/ingestion/curves` | Persist a zero-curve snapshot, creating the curve on first use; `{"curve_id","inserted"}` |
| POST | `/v1/ingestion/trades` | Explicit 501 stub |

Quote request:

```json
{
  "instrument_id": "00000000-0000-0000-0000-000000000001",
  "source_id": "sample",
  "observed_at": "2026-09-16T12:01:00Z",
  "bid": "201.25",
  "ask": "201.35"
}
```

Amounts/rates are JSON decimal strings, not floats. Quote prices allow up to 12 integral and 8 fractional digits. `received_at` is assigned by PostgreSQL and cannot be supplied by a caller. Timestamps use RFC 3339, dates ISO 8601. Unknown JSON fields, extra JSON documents, malformed IDs, crossed/negative quotes, and unknown instrument/source references are rejected. Error JSON is `{"error":"..."}` with 400/401/403/404/409/429/500; unimplemented ingestion returns 501. Collection endpoints return empty arrays when no matching data exists. Missing option quotes use empty bid/ask/source strings in v0.1.

Quotes are append-only by `(instrument_id, source_id, observed_at)`; replay keeps the first payload, including when a conflicting payload is submitted at the same key. Latest ordering is observation time, receipt time, then internal ID; no vendor preference is implied. Future correction/revision support should introduce explicit revision semantics.

Reference validity intervals and separate observation/receipt times establish temporal boundaries, but this is **not a full bitemporal reference database**. Historical as-of selection, reference revision history, source prioritization, pagination beyond the search cap, calendars, corporate actions, streaming, and live feeds are future work. Curve rates are decimal continuously compounded zero rates for demonstration; real FRED observations require a deliberate curve construction process, not direct substitution.

## Pricing

```cpp
#include <pricing/black_scholes.hpp>
auto result = pricing::black_scholes(
    {pricing::OptionRight::Call, 100.0, 1.0},
    {100.0, 0.20, 0.05, 0.0});
// result.price ≈ 10.450583572, per unit (apply contract multiplier separately)
```

The same engine is available over HTTP:

```sh
curl -X POST http://127.0.0.1:8080/v1/pricing/european \
  -H 'Content-Type: application/json' \
  -d '{"exercise_style":"european","right_type":"call","strike":"100","time_to_expiry":"1","spot":"100","volatility":"0.20","risk_free_rate":"0.05","dividend_yield":"0"}'
```

Response: `{"model":"black_scholes","price":"10.450583572185565"}` (last digits may vary by platform).

All eight fields are required. `exercise_style` must be `european`; `right_type` is `call` or `put`. Numeric inputs are decimal strings with up to 12 integral and 8 fractional digits, without whitespace or exponent notation. Strike and spot must be positive; expiry and volatility may be zero; rates and dividend yield may be negative. Volatility `0.20` means 20% annualized, and rate `0.05` means 5% continuously compounded. American exercise is rejected.

This endpoint prices explicit scenarios: it does not look up instruments, pick quote sides, construct curves, infer a valuation date/day-count convention, apply contract multipliers, or persist results. The result is in the same currency units as spot and strike. The C++ model uses binary floating-point; a decimal-string response preserves the computed double for transport, not exact decimal arithmetic or guaranteed decimal precision. Invalid requests and numerical overflow return 400; unexpected native failures return a generic 500. Requests are limited to 16 KiB and reject unknown fields or extra JSON documents. The frontend scenario form calls this endpoint through `PricingService` and a component-scoped `PricingStore`.

European options only; continuously compounded rates/dividend yield and time in years. Handles expiry and zero volatility explicitly. Tests cover reference call/put values, dividend-adjusted put-call parity, expiry payoff, deterministic zero volatility, and invalid inputs. Synthetic AAPL contracts are intentionally European for this demo; they do not represent actual listed AAPL contract specifications.

## Security master, price history and analytics

Analytics are pure Go (`internal/analytics`), independent of persistence and transport; `service.Securities` validates decimal-string inputs and formats float64 results as decimal strings, like the option pricer.

**Security master.** Instruments are created with one to twenty identifiers (`scheme` is lowercase, e.g. `ticker`, `isin`, `cusip`, `figi`); ISIN and CUSIP check digits are verified. Bond terms: `coupon_rate` is a decimal fraction (`"0.0425"` = 4.25%), `coupon_frequency` is 0 (zero coupon), 1, 2, 4 or 12, `day_count` is `30/360` (US bond basis), `ACT/360`, `ACT/365F` or `ACT/ACT` (ICMA), and `redemption` and call prices are per 100 of face (redemption defaults to 100). `face_value` is the denomination and does not scale analytics.

```sh
curl -X POST http://127.0.0.1:8080/v1/instruments/bonds -H 'Content-Type: application/json' -d '{
  "name":"Apple 3.85% 2043","currency":"USD","issuer":"Apple Inc.","issue_date":"2013-05-03","maturity":"2043-05-04",
  "coupon_rate":"0.0385","coupon_frequency":2,"day_count":"30/360","face_value":"1000",
  "calls":[{"date":"2042-11-04","price":"100"}],"identifiers":[{"scheme":"cusip","value":"037833AL4","venue":""}]}'
```

**Price history.** `price_bar` holds one daily bar per instrument, source and date. Equity bars are in currency units; bond bars are clean prices per 100. `adj_close` is optional and vendor-supplied. Like quotes, bars are append-only: replaying a key keeps the first payload, and the response's `inserted` count says how many were new. Every history read names its `source`; sources are never mixed or prioritized silently.

**Bond analytics.** These cover fixed-rate bullet bonds with discrete calls. Coupon dates are generated backward from maturity, with the end-of-month rule when maturity falls on a month end. Settlement must be on or after issue and before maturity. Results: clean, dirty and accrued prices per 100, yield to maturity (compounded at the coupon frequency, semiannual for zeros), yield to worst across maturity and future call dates (a call between coupon dates pays the accrued coupon), workout date, Macaulay and modified duration in years, convexity, DV01 per 100 face, the remaining cash flows, and optionally a Z-spread. The Z-spread is a constant spread over the curve's continuously compounded zero rates, interpolated linearly in tenor days with flat extrapolation; tenors are measured from settlement. Durations and convexity are computed to maturity. Not modelled: odd first or last coupons (accrual starts at issue but the first coupon is regular), business-day adjustment, ex-dividend periods, floating rates, sinking funds, and option-adjusted spread.

```sh
curl -X POST http://127.0.0.1:8080/v1/analytics/bond -H 'Content-Type: application/json' -d '{
  "terms":{"maturity":"2046-01-15","coupon_rate":"0.06","coupon_frequency":2,"day_count":"30/360"},
  "settlement":"2026-01-15","yield":"0.08","curve":[{"tenor_days":365,"zero_rate":"0.04"}]}'
# clean_price ≈ 80.2072261, modified_duration ≈ 10.5023952, convexity ≈ 162.319493
```

**Performance.** Statistics use simple daily returns over the bars in `[from, to]`, annualized with 252 trading days: total and annualized (calendar CAGR) return, annualized volatility, Sharpe ratio against `risk_free_rate` (an annual decimal, default 0), maximum drawdown, and the latest 20, 50 and 200-bar simple moving averages when there is enough history. Pass `benchmark_id` to add beta and correlation, aligned on dates both series share. `adj_close` is used only when every bar of both series has it (`price_field` in the response says which). Valuation ratios need a fundamentals feed and are future work.

## Validation

`make test` runs Go, Python and C++ tests; `go test -race ./...` and `go vet ./...` provide additional Go checks. Go HTTP tests exercise the real C++ engine without PostgreSQL: reference call/put prices, dividend parity, expiry, zero volatility, negative rates, concurrent calls, validation, overflow recovery, and internal error responses. `make smoke` requires the running local API and seeded database. Frontend verification: `cd web && npm test -- --watch=false && npm run build`. Frontend tests cover cancellation, error recovery, dependent state reset, paired strike rows, curve loading/cancellation/retry, explicit rate copying and currency restrictions, contract selection and resets, explicit quote copying, date-only ACT/365F (including leap years and invalid dates), pricing form validation, exact decimal transport/rendering, reset, and component destruction. Browser checks exercised the real Go API and PostgreSQL.
