import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { provideHttpClientTesting, HttpTestingController } from '@angular/common/http/testing';
import { beforeEach, afterEach, describe, it, expect } from 'vitest';
import { CurveWorkspaceStore } from './curve-workspace.store';

const curves = [
  { id: 'a', name: 'Curve A', currency: 'USD' },
  { id: 'b', name: 'Curve B', currency: 'USD' },
];
const point = {
  tenor_days: 365,
  zero_rate: '-0.01234567',
  source_id: 'sample',
  observed_at: '2026-09-15T12:00:00Z',
};
describe('CurveWorkspaceStore', () => {
  let http: HttpTestingController;
  let store: InstanceType<typeof CurveWorkspaceStore>;
  beforeEach(() => {
    TestBed.configureTestingModule({
      providers: [provideHttpClient(), provideHttpClientTesting(), CurveWorkspaceStore],
    });
    http = TestBed.inject(HttpTestingController);
    store = TestBed.inject(CurveWorkspaceStore);
  });
  afterEach(() => {
    http.verify();
    TestBed.resetTestingModule();
  });
  const load = () => {
    store.loadCurrency('usd');
    http.expectOne('/v1/curves?currency=USD').flush(curves);
  };
  it('loads a currency without automatically selecting a curve and preserves decimals', () => {
    load();
    expect(store.selectedCurve()).toBeNull();
    expect(store.curvesRequest().isLoaded).toBe(true);
    store.selectCurve('a');
    http.expectOne('/v1/curves/a/points').flush([point]);
    expect(store.selectedCurve()?.id).toBe('a');
    expect(store.points()[0].zero_rate).toBe('-0.01234567');
    expect(store.pointsRequest().isLoaded).toBe(true);
  });
  it('cancels old point reads when switching curves or currencies', () => {
    load();
    store.selectCurve('a');
    const a = http.expectOne('/v1/curves/a/points');
    store.selectCurve('b');
    expect(a.cancelled).toBe(true);
    expect(store.pointsRequest().isLoading).toBe(true);
    const b = http.expectOne('/v1/curves/b/points');
    store.loadCurrency('EUR');
    expect(b.cancelled).toBe(true);
    expect(store.selectedCurve()).toBeNull();
    expect(store.points()).toEqual([]);
    const eur = http.expectOne('/v1/curves?currency=EUR');
    store.loadCurrency('JPY');
    expect(eur.cancelled).toBe(true);
    expect(store.curvesRequest().isLoading).toBe(true);
    http.expectOne('/v1/curves?currency=JPY').flush([]);
    expect(store.curvesRequest().isLoaded).toBe(true);
  });
  it('clears old snapshots on refresh and recovers from point/list errors', () => {
    load();
    store.selectCurve('a');
    http.expectOne('/v1/curves/a/points').flush([point]);
    store.selectCurve('a');
    expect(store.points()).toEqual([]);
    http.expectOne('/v1/curves/a/points').flush({}, { status: 500, statusText: 'Failure' });
    expect(store.pointsRequest().error).toBeTruthy();
    expect(store.pointsRequest().isLoaded).toBe(false);
    store.selectCurve('a');
    http.expectOne('/v1/curves/a/points').flush([]);
    expect(store.pointsRequest().error).toBeNull();
    expect(store.pointsRequest().isLoaded).toBe(true);
    store.loadCurrency('USD');
    http.expectOne('/v1/curves?currency=USD').error(new ProgressEvent('error'));
    expect(store.curvesRequest().error).toBeTruthy();
    load();
    expect(store.curvesRequest().error).toBeNull();
  });
  it('rejects unknown and mismatched curves and cancels all work on clear', () => {
    store.loadCurrency('USD');
    http
      .expectOne('/v1/curves?currency=USD')
      .flush([...curves, { id: 'eur', name: 'EUR', currency: 'EUR' }]);
    store.selectCurve('missing');
    store.selectCurve('eur');
    http.expectNone('/v1/curves/eur/points');
    expect(store.selectedCurve()).toBeNull();
    store.selectCurve('a');
    const pending = http.expectOne('/v1/curves/a/points');
    store.clear();
    expect(pending.cancelled).toBe(true);
    expect(store.curves()).toEqual([]);
    expect(store.pointsRequest().isLoading).toBe(false);
    store.loadCurrency('invalid');
    http.expectNone((request) => request.url === '/v1/curves');
    expect(store.curvesRequest().error).toBeTruthy();
  });
});
