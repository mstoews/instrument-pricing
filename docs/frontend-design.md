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

Current validation: 47 frontend tests and the production build pass. Integration tests exercise chain selection through the form and HTTP request, unsupported exercise styles, missing quotes, precision, same-contract reselection, context cancellation, and date edge cases. Browser inventory was checked again and remained empty; visual desktop/mobile QA is outstanding.

Next work: visual QA with a connected browser, then a yield-curve workspace and explicit curve-based rate selection. No curve construction or automatic rate selection is implemented.
