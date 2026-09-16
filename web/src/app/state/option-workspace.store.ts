import { computed, inject } from '@angular/core';
import { patchState, signalStore, withComputed, withMethods, withState } from '@ngrx/signals';
import { rxMethod } from '@ngrx/signals/rxjs-interop';
import { tapResponse } from '@ngrx/operators';
import { EMPTY, merge, pipe, switchMap } from 'rxjs';
import { MarketDataService } from '../core/market-data';
import { OptionsService } from '../core/options';
import { Instrument, Quote, OptionContract, RequestState, idle, pending, loaded, failed } from '../core/models';
interface WorkspaceState {
 instrument: Instrument | null; quote: Quote | null; expirations: string[]; expiration: string;
 chain: OptionContract[]; quoteRequest: RequestState; expirationsRequest: RequestState; chainRequest: RequestState;
}
export const OptionWorkspaceStore = signalStore(
 withState<WorkspaceState>({ instrument: null, quote: null, expirations: [], expiration: '', chain: [], quoteRequest: idle(), expirationsRequest: idle(), chainRequest: idle() }),
 withComputed(store => ({
  rows: computed(() => {
   const rows = new Map<string, { strike: string; call?: OptionContract; put?: OptionContract }>();
   for (const option of store.chain()) {
    const row = rows.get(option.strike) ?? { strike: option.strike };
    row[option.right_type] = option; rows.set(option.strike, row);
   }
   return [...rows.values()].sort((a,b) => Number(a.strike) - Number(b.strike));
  }),
 })),
 withMethods((store, api = inject(OptionsService)) => ({
  loadChain: rxMethod<string>(pipe(switchMap(expiration => {
   patchState(store, { expiration, chain: [], chainRequest: idle() });
   const id = store.instrument()?.id;
   if (!id || !expiration) return EMPTY;
   patchState(store, { chainRequest: pending() });
   return api.chain(id, expiration).pipe(tapResponse({
    next: chain => patchState(store, { chain, chainRequest: loaded() }),
    error: error => patchState(store, { chainRequest: failed(error) }),
   }));
  }))),
 })),
 withMethods((store, quotes = inject(MarketDataService), options = inject(OptionsService)) => ({
  select: rxMethod<Instrument>(pipe(switchMap(instrument => {
   store.loadChain(''); // Cancel the old chain before selecting another instrument.
   patchState(store, { instrument, quote: null, expirations: [], quoteRequest: pending(), expirationsRequest: pending() });
   return merge(
    quotes.quote(instrument.id).pipe(tapResponse({
     next: quote => patchState(store, { quote, quoteRequest: loaded() }),
     error: error => patchState(store, { quoteRequest: failed(error) }),
    })),
    options.expirations(instrument.id).pipe(tapResponse({
     next: expirations => { patchState(store, { expirations, expirationsRequest: loaded() }); store.loadChain(expirations[0] ?? ''); },
     error: error => patchState(store, { expirationsRequest: failed(error) }),
    })),
   );
  }))),
 }))
);
