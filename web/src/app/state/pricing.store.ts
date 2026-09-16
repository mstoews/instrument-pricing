import { inject } from '@angular/core';
import { HttpErrorResponse } from '@angular/common/http';
import { patchState, signalStore, withMethods, withState } from '@ngrx/signals';
import { rxMethod } from '@ngrx/signals/rxjs-interop';
import { tapResponse } from '@ngrx/operators';
import { EMPTY, pipe, switchMap } from 'rxjs';
import { PricingService } from '../core/pricing';
import {
  EuropeanPriceRequest,
  EuropeanPriceResult,
  RequestState,
  idle,
  pending,
  loaded,
} from '../core/models';

interface PricingState {
  result: EuropeanPriceResult | null;
  request: RequestState;
}

function pricingError(error: unknown): RequestState {
  let message = 'Calculation failed. Please retry.';
  if (error instanceof HttpErrorResponse) {
    if (error.status === 0)
      message = 'Cannot reach the pricing service. Check your connection and retry.';
    else if (error.status === 400 && typeof error.error?.error === 'string')
      message = error.error.error;
  }
  return { ...idle(), error: message };
}

export const PricingStore = signalStore(
  withState<PricingState>({ result: null, request: idle() }),
  withMethods((store, api = inject(PricingService)) => ({
    // This POST is a stateless calculation. Latest scenario wins; no data is written.
    _calculate: rxMethod<EuropeanPriceRequest | null>(
      pipe(
        switchMap((scenario) => {
          patchState(store, { result: null, request: idle() });
          if (!scenario) return EMPTY;
          patchState(store, { request: pending() });
          return api.european(scenario).pipe(
            tapResponse({
              next: (result) => patchState(store, { result, request: loaded() }),
              error: (error) => patchState(store, { request: pricingError(error) }),
            }),
          );
        }),
      ),
    ),
  })),
  withMethods((store) => ({
    calculate(scenario: EuropeanPriceRequest) {
      store._calculate({ ...scenario });
    },
    clear() {
      store._calculate(null);
    },
  })),
);
