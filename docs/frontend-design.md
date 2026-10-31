# Frontend design — implemented

User-specified separation:

1. HTTP services make server calls and return typed observables.
2. NgRx Signal Stores inject services, trigger requests, and maintain state.
3. Components invoke store methods and read signals for rendering.

Follow the supplied AccountsStore pattern: typed initial state, withState, withComputed, withMethods, rxMethod, patchState, tapResponse, and deliberate initialization hooks.

Implemented first slice:

- InstrumentService: search and identifier resolution.
- MarketDataService: latest quote.
- OptionsService: expirations and chain.
- InstrumentStore: search results, selected instrument and search request status.
- OptionWorkspaceStore: quote, expirations, selected expiry and chain; independent loading/error state for each request.
- Search and option-chain components: call store methods and display signals. No direct HttpClient calls or component-owned copies of server state.

Use switchMap for changing read selections so stale results cannot replace the active instrument. Set loading inside the switched request to avoid cancelled-request finalizers clearing a newer request's flag. Set loaded only on success, derive projections with computed, and clear dependent state when instrument/expiry changes. Choose write concurrency separately when writes are introduced.

Market-data vendors remain behind Python adapters and Go REST. Browser services do not acquire market vendor credentials.

Stores are provided at the workspace component boundary so state is disposed when the workspace is destroyed. HTTP services are root singletons. Store tests use Angular HttpTestingController to verify cancellations and recovery. Components use OnPush change detection and native accessible form controls.

## European option pricing — implemented

- `PricingService` posts typed decimal-string scenarios to `POST /v1/pricing/european`.
- `PricingStore` owns the result and loading/error state. Its `rxMethod` uses `switchMap`: this POST is a stateless calculation, so the latest scenario wins. Changing inputs or resetting sends a cancellation through the same pipeline and clears stale results immediately. The store is scoped to the pricing component and cancels requests on destruction.
- `EuropeanPricer` uses Angular Signal Forms for the editable draft and local validation, calls store methods, and renders store signals. All numeric controls are text fields so entered decimal strings retain their precision. There are no direct component HTTP calls or component-owned server state.
- The panel below the market workspace starts with the documented sample scenario. Its initial sample inputs are independent of instrument selection and quotes; the explicit contract workflow below can seed a new draft. Users choose call/put and enter spot, strike, time in years, annualized volatility, and continuously compounded rate/dividend yield. Rates use decimal fractions (0.05 = 5%). Only European exercise is supported.
- Field validation mirrors the REST decimal format and bounds, allowing zero expiry/volatility and negative rates. The server remains authoritative for numerical range errors. Results are displayed as the returned decimal string, per unit; no contract multiplier or currency conversion is applied.
- Controls have labels, field errors, loading feedback, retry, and reset. The output uses a live region. Editing any field invalidates the result; retry preserves the draft.

Verification for the initial pricing panel: all 24 frontend tests and the production build passed. DOM tests cover sample and put submission, precision, validation, edit/reset cancellation, API error recovery, and disposal. The running frontend proxy was checked against the real Go/C++ endpoint for call/put prices and numerical range errors. Visual browser QA could not run because no browser was available in the session.

## Contract-to-scenario workflow — implemented

- The option chain has **Use call** / **Use put** actions, disabled for non-European contracts. `OptionWorkspaceStore` owns the selected contract, restricts selection to current chain members, and clears it when the instrument or expiry changes or quotes are refreshed. Selecting a contract again deliberately starts a fresh draft.
- Contract, underlying instrument, and quote flow to `EuropeanPricer` through signal inputs. Its input-change lifecycle resets the editable draft and cancels the prior calculation through `PricingStore`. Server state stays in stores. Selecting a contract focuses the pricer heading for keyboard navigation.
- Contract strike and right are copied exactly. Spot, year fraction, volatility, rate, and dividend yield start empty; there are no implicit market assumptions. All scenario fields remain editable, and the heading identifies the starting contract.
- Underlying bid/ask buttons copy the chosen decimal string explicitly. The quote source and observed time are shown; the quote may not match the chosen valuation date. If quotes are absent, spot can be entered manually. Editing a copied spot clears its provenance label.
- The optional valuation-date helper uses UTC calendar dates and ACT/365F: days between valuation and expiration divided by 365, rounded to eight decimal places. Same-day expiry gives zero. Invalid dates and dates after expiration cannot be applied. Exchange closing times, holidays, and time-of-day are not inferred. Changing an applied valuation date clears the generated year fraction; users can also enter a year fraction manually.
- Resetting contract inputs restores strike/right and clears market assumptions. **Use sample scenario** leaves contract mode and restores the original sample.

Contract-workflow validation: 47 frontend tests and the production build passed. Integration tests exercise chain selection through the form and HTTP request, unsupported exercise styles, missing quotes, precision, same-contract reselection, context cancellation, and date edge cases. Browser inventory was checked again and remained empty; visual desktop/mobile QA is outstanding.

## Yield-curve workspace — implemented

- `CurvesService` exposes typed curve-list and snapshot-point reads. `CurveWorkspaceStore` owns currency, curve selection, point data, and independent loading/error states. Switching currency cancels list and point reads; selecting or refreshing a curve clears its old points. Unknown or mismatched curve selections are rejected.
- `CurveWorkspace` is scoped beneath the pricer, using a component-scoped store and Signal Forms for currency input. Requests start only when the user loads curves. Its expandable panel includes currency selection, curve choices, refresh/retry, empty/error states, and a table with tenor days, exact decimal zero rate, source, and UTC observation time.
- Linked contract currency is read-only; standalone scenarios let users choose a three-letter currency. Currency changes clear browsing state and cancel pending requests. Component destruction cancels outstanding reads.
- **Use rate** emits a current-snapshot point to the pricer. It copies the exact decimal rate, cancels pending pricing, clears the previous price, and records a provenance label identifying currency, curve, tenor, source, and observed time. Zero and negative rates are supported. Rates outside the pricing API decimal format stay visible with copying disabled; no silent rounding is performed.
- Copied rates are fixed scenario assumptions. Refreshing curves or browsing another currency leaves a previously copied rate and its provenance unchanged. Editing the rate clears its provenance; resetting or selecting a new contract clears the draft as before.
- A selected point is treated as a flat scenario rate, with no interpolation, tenor matching, historical as-of alignment, curve construction, or vendor access. These limits are displayed alongside the controls.

Current verification: 60 frontend tests and the production build pass. Tests cover curve/currency cancellation, stale snapshot prevention, errors and empty states, exact negative/zero rate copying, unsupported precision, contract currency restrictions, provenance, refresh behavior, and disposal. Visual desktop/mobile QA remains outstanding because no browser was connected during prior checks.

## Authentication — implemented

- `AuthService` wraps `/v1/auth/login`, `/refresh` and `/logout`. `AuthStore` is a root store (the session outlives any workspace) holding the user, the in-memory access token and its expiry, sign-in request state, and a notice for expired or remote sign-outs.
- Routing: `/login` (guest guard) and the workspace at `/` (auth guard), both lazy-loaded. An app initializer restores a stored session through one refresh before the first navigation, so both guards decide synchronously. A rejected stored token is discarded; a network failure keeps it for the next load.
- `authInterceptor` handles `/v1` calls except the auth endpoints. It attaches the token and refreshes it 30 seconds before expiry. On a 401 it shares one refresh across all failing requests and retries each once. A rejected refresh ends the session: the store clears tokens and redirects once to `/login?returnUrl=…` with a notice. Errors other than 401 pass through unchanged. Stores keep their existing error handling.
- Refresh tokens are single-use on the server, so `refresh()` is single-flight within a tab and runs under a Web Locks lock across tabs. Inside the lock it re-reads storage, so a token another tab just rotated is used rather than replayed. Rotation overwrites the stored key without removing it, because a removal is the cross-tab signal for sign-out.
- `Login` uses Signal Forms (required, email and length validators) with labelled fields, `autocomplete` hints for password managers, inline errors and a live alert for server errors. Repeat submits are ignored while pending (`exhaustMap`); editing clears a stale error. 401, 429 and network failures each get a specific message.
- Sign-out sends the refresh token to the server, so it works even when the access token has expired or been rotated by another tab. It clears local state first, so it never depends on the network.

Verification: 84 frontend tests and the production build pass. A Playwright run against the real Go API and PostgreSQL covered redirect-to-login, validation, wrong password, sign-in, authorized data loading, reload restore, silent refresh before expiry (exactly one refresh), a server-rejected refresh ending the session, remember-me storage, a second tab sharing the session, sign-out propagating across tabs and revoking the session server-side, and a 390 px mobile layout with no overflow.

Next work: visual QA with a connected browser and deliberate maturity-matched curve interpolation, with explicit day-count and extrapolation rules. No automatic curve construction or live vendor integration is implemented.
