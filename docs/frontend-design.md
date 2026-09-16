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
