import { computed, inject } from '@angular/core';
import { patchState, signalStore, withComputed, withMethods, withState } from '@ngrx/signals';
import { rxMethod } from '@ngrx/signals/rxjs-interop';
import { tapResponse } from '@ngrx/operators';
import { EMPTY, pipe, switchMap } from 'rxjs';
import { CurvesService } from '../core/curves';
import { Curve, CurvePoint, RequestState, idle, pending, loaded } from '../core/models';

interface CurveState {
  currency: string;
  curves: Curve[];
  selectedId: string;
  points: CurvePoint[];
  curvesRequest: RequestState;
  pointsRequest: RequestState;
}
export const CurveWorkspaceStore = signalStore(
  withState<CurveState>({
    currency: '',
    curves: [],
    selectedId: '',
    points: [],
    curvesRequest: idle(),
    pointsRequest: idle(),
  }),
  withComputed((store) => ({
    selectedCurve: computed(
      () => store.curves().find((curve) => curve.id === store.selectedId()) ?? null,
    ),
  })),
  withMethods((store, api = inject(CurvesService)) => ({
    selectCurve: rxMethod<string>(
      pipe(
        switchMap((id) => {
          const curve = store
            .curves()
            .find((item) => item.id === id && item.currency === store.currency());
          patchState(store, { selectedId: curve?.id ?? '', points: [], pointsRequest: idle() });
          if (!curve) return EMPTY;
          patchState(store, { pointsRequest: pending() });
          return api.points(curve.id).pipe(
            tapResponse({
              next: (points) => patchState(store, { points, pointsRequest: loaded() }),
              error: () =>
                patchState(store, {
                  pointsRequest: { ...idle(), error: 'Unable to load curve points. Please retry.' },
                }),
            }),
          );
        }),
      ),
    ),
  })),
  withMethods((store, api = inject(CurvesService)) => ({
    loadCurrency: rxMethod<string>(
      pipe(
        switchMap((value) => {
          store.selectCurve('');
          const currency = value.trim().toUpperCase();
          patchState(store, { currency, curves: [], curvesRequest: idle() });
          if (!currency) return EMPTY;
          if (!/^[A-Z]{3}$/.test(currency)) {
            patchState(store, {
              curvesRequest: { ...idle(), error: 'Enter a three-letter currency code.' },
            });
            return EMPTY;
          }
          patchState(store, { curvesRequest: pending() });
          return api.curves(currency).pipe(
            tapResponse({
              next: (curves) => patchState(store, { curves, curvesRequest: loaded() }),
              error: () =>
                patchState(store, {
                  curvesRequest: { ...idle(), error: 'Unable to load curves. Please retry.' },
                }),
            }),
          );
        }),
      ),
    ),
  })),
  withMethods((store) => ({
    clear() {
      store.loadCurrency('');
    },
  })),
);
