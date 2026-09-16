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
make api
```

In another terminal in this directory:

```sh
make sample
make smoke
make test
```

`make sample` submits a synthetic quote with the current observation time. `make smoke` verifies resolution, search, option expirations/chain, curves, ingestion, decimal precision, idempotency, late arrival ordering, pricing through the C++ engine, and validation over HTTP against real PostgreSQL. It adds synthetic quotes to the local sample database.

API: `http://127.0.0.1:8080`. Postgres: `127.0.0.1:54329`, database/user/password `pricing`. These are local development defaults. Override `DATABASE_URL` and `HTTP_ADDR` as needed. The API is unauthenticated and binds to loopback by default; add authentication before exposing it beyond local development. Stop API with Ctrl-C and database with `docker compose stop` (data is retained).

```sh
curl 'http://127.0.0.1:8080/v1/instruments/resolve?scheme=ticker&value=AAPL&venue=XNAS'
curl 'http://127.0.0.1:8080/v1/options/00000000-0000-0000-0000-000000000001/chain?expiration=2027-01-15'
```

To install Python as a package, optionally create a virtual environment and run `pip install -e ./ingestion`; then use `pricing-ingest`. The Make targets need no Python dependencies.

## Run the frontend

Use Node.js 24 LTS or another Angular 22 supported release and npm. In a third terminal, with the API running:

```sh
cd web
npm ci
npm start
```

Open http://127.0.0.1:4200 and select **Apple Inc. (sample)**. Development requests to `/v1` are proxied to the local Go API. For production hosting, route `/v1` to Go on the same origin; the development proxy is not included in the build. `npm run build` writes to `web/dist/web`.

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
- `ingestion`: provider/sink protocols, normalized decimal quote model and REST client. Massive, FINRA, FRED adapters explicitly raise `NotImplementedError`. No vendor payloads or credentials cross into Go/C++; Python never accesses PostgreSQL.
- `pricing`: a C++ library with no persistence, network or vendor dependency. The Go adapter in `pricing/src` implements `domain.PricingEngine`; a C ABI catches all native exceptions before returning to Go. The adapter lives alongside the canonical C++ source so Go builds it directly and tracks source changes. `cmd/api` injects it into `service.Pricer`. CMake still builds and tests the standalone C++ library.

## REST contract

| Method | Path | Result |
| --- | --- | --- |
| GET | `/healthz` | Process liveness |
| GET | `/v1/instruments?q=AAPL` | Up to 50 current active instruments; deterministic name/ID order |
| GET | `/v1/instruments/resolve?scheme=ticker&value=AAPL&venue=XNAS` | Exact, case-sensitive identifier matches; array preserves ambiguity |
| GET | `/v1/market-data/{id}/quote` | Latest observed quote, 404 if absent |
| GET | `/v1/options/{underlyingId}/expirations` | Sorted date strings; includes historical expirations |
| GET | `/v1/options/{underlyingId}/chain?expiration=2027-01-15` | Contracts sorted by strike/right with latest available quote |
| GET | `/v1/curves?currency=USD` | Curve definitions |
| GET | `/v1/curves/{id}/points` | Latest snapshot points, sorted by tenor days |
| POST | `/v1/pricing/european` | Stateless European Black-Scholes scenario price, per unit |
| POST | `/v1/ingestion/quotes` | Persist normalized quote; 200 accepted, including identical replay |
| POST | `/v1/ingestion/instruments`, `/trades`, `/curves` | Explicit 501 stubs |

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

Amounts/rates are JSON decimal strings, not floats. Quote prices allow up to 12 integral and 8 fractional digits. `received_at` is assigned by PostgreSQL and cannot be supplied by a caller. Timestamps use RFC 3339, dates ISO 8601. Unknown JSON fields, extra JSON documents, malformed IDs, crossed/negative quotes, and unknown instrument/source references are rejected. Error JSON is `{"error":"..."}` with 400/404/500; unimplemented ingestion returns 501. Collection endpoints return empty arrays when no matching data exists. Missing option quotes use empty bid/ask/source strings in v0.1.

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

## Validation

`make test` runs Go, Python and C++ tests; `go test -race ./...` and `go vet ./...` provide additional Go checks. Go HTTP tests exercise the real C++ engine without PostgreSQL: reference call/put prices, dividend parity, expiry, zero volatility, negative rates, concurrent calls, validation, overflow recovery, and internal error responses. `make smoke` requires the running local API and seeded database. Frontend verification: `cd web && npm test -- --watch=false && npm run build`. Frontend tests cover cancellation, error recovery, dependent state reset, paired strike rows, curve loading/cancellation/retry, explicit rate copying and currency restrictions, contract selection and resets, explicit quote copying, date-only ACT/365F (including leap years and invalid dates), pricing form validation, exact decimal transport/rendering, reset, and component destruction. Browser checks exercised the real Go API and PostgreSQL.
