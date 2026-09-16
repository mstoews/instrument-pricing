import { inject } from '@angular/core';
import { patchState, signalStore, withMethods, withState } from '@ngrx/signals';
import { rxMethod } from '@ngrx/signals/rxjs-interop';
import { tapResponse } from '@ngrx/operators';
import { pipe, switchMap } from 'rxjs';
import { InstrumentService } from '../core/instrument';
import { Instrument, RequestState, idle, pending, loaded, failed } from '../core/models';
interface InstrumentState { results: Instrument[]; selected: Instrument | null; query: string; request: RequestState }
export const InstrumentStore = signalStore(
 withState<InstrumentState>({ results: [], selected: null, query: 'AAPL', request: idle() }),
 withMethods((store, api = inject(InstrumentService)) => ({
  select(instrument: Instrument) { patchState(store, { selected: instrument }); },
  search: rxMethod<string>(pipe(switchMap(query => {
   patchState(store, { query, results: [], request: pending() });
   return api.search(query).pipe(tapResponse({
    next: results => patchState(store, { results, request: loaded() }),
    error: error => patchState(store, { request: failed(error) }),
   }));
  }))),
 }))
);
