import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { provideHttpClientTesting, HttpTestingController } from '@angular/common/http/testing';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { EuropeanPriceRequest } from '../core/models';
import { PricingStore } from './pricing.store';

const scenario: EuropeanPriceRequest = {
  exercise_style: 'european',
  right_type: 'call',
  strike: '100.12345678',
  time_to_expiry: '1',
  spot: '100',
  volatility: '0.20',
  risk_free_rate: '-0.05',
  dividend_yield: '0',
};
const endpoint = '/v1/pricing/european';

describe('PricingStore', () => {
  let http: HttpTestingController;
  let store: InstanceType<typeof PricingStore>;
  beforeEach(() => {
    TestBed.configureTestingModule({
      providers: [provideHttpClient(), provideHttpClientTesting(), PricingStore],
    });
    http = TestBed.inject(HttpTestingController);
    store = TestBed.inject(PricingStore);
  });
  afterEach(() => {
    http.verify();
    TestBed.resetTestingModule();
  });

  it('posts decimal strings and preserves the returned price', () => {
    expect(store.result()).toBeNull();
    store.calculate(scenario);
    expect(store.request().isLoading).toBe(true);
    const req = http.expectOne(endpoint);
    expect(req.request.method).toBe('POST');
    expect(req.request.body).toEqual(scenario);
    req.flush({ model: 'black_scholes', price: '10.450583572185565' });
    expect(store.result()?.price).toBe('10.450583572185565');
    expect(store.request()).toEqual({ isLoading: false, isLoaded: true, error: null });
  });

  it('cancels superseded calculations without clearing the new loading flag', () => {
    store.calculate(scenario);
    const old = http.expectOne(endpoint);
    store.calculate({ ...scenario, right_type: 'put' });
    expect(old.cancelled).toBe(true);
    expect(store.request().isLoading).toBe(true);
    const current = http.expectOne(endpoint);
    expect(current.request.body.right_type).toBe('put');
    current.flush({ model: 'black_scholes', price: '5.573526022' });
    expect(store.result()?.price).toBe('5.573526022');
  });

  it('clears completed results and cancels pending work when inputs change', () => {
    store.calculate(scenario);
    http.expectOne(endpoint).flush({ model: 'black_scholes', price: '10' });
    store.clear();
    expect(store.result()).toBeNull();
    expect(store.request().isLoaded).toBe(false);
    store.calculate(scenario);
    const pending = http.expectOne(endpoint);
    store.clear();
    expect(pending.cancelled).toBe(true);
    expect(store.request()).toEqual({ isLoading: false, isLoaded: false, error: null });
  });

  it('shows API validation errors and recovers on retry', () => {
    store.calculate(scenario);
    http
      .expectOne(endpoint)
      .flush(
        { error: 'Pricing inputs exceed the model range' },
        { status: 400, statusText: 'Bad Request' },
      );
    expect(store.request().error).toBe('Pricing inputs exceed the model range');
    expect(store.request().isLoaded).toBe(false);
    store.calculate(scenario);
    expect(store.request().error).toBeNull();
    http.expectOne(endpoint).flush({ model: 'black_scholes', price: '0' });
    expect(store.result()?.price).toBe('0');
    expect(store.request().isLoaded).toBe(true);
  });

  it('provides useful network and server failure messages', () => {
    store.calculate(scenario);
    http.expectOne(endpoint).error(new ProgressEvent('error'));
    expect(store.request().error).toContain('Cannot reach');
    store.calculate(scenario);
    http
      .expectOne(endpoint)
      .flush({ error: 'private details' }, { status: 500, statusText: 'Failure' });
    expect(store.request().error).toBe('Calculation failed. Please retry.');
    expect(store.result()).toBeNull();
  });
});
