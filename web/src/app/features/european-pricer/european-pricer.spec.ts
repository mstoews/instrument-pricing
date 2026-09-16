import { ComponentFixture, TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { provideHttpClientTesting, HttpTestingController } from '@angular/common/http/testing';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { EuropeanPricer } from './european-pricer';

const endpoint = '/v1/pricing/european';
describe('EuropeanPricer', () => {
  let fixture: ComponentFixture<EuropeanPricer>;
  let http: HttpTestingController;
  let root: HTMLElement;
  beforeEach(async () => {
    TestBed.configureTestingModule({
      imports: [EuropeanPricer],
      providers: [provideHttpClient(), provideHttpClientTesting()],
    });
    http = TestBed.inject(HttpTestingController);
    fixture = TestBed.createComponent(EuropeanPricer);
    root = fixture.nativeElement;
    await fixture.whenStable();
  });
  afterEach(() => {
    http.verify();
    TestBed.resetTestingModule();
  });
  const input = (key: string, value: string) => {
    const field = root.querySelector<HTMLInputElement>('#pricing-' + key)!;
    field.value = value;
    field.dispatchEvent(new Event('input', { bubbles: true }));
    field.dispatchEvent(new Event('blur'));
  };
  const calculate = async () => {
    root
      .querySelector('form')!
      .dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));
    await Promise.resolve();
  };

  it('submits the sample and renders the exact decimal response', async () => {
    expect(root.textContent).toContain('independent of the selected instrument');
    http.expectNone(endpoint);
    await calculate();
    const request = http.expectOne(endpoint);
    expect(request.request.body).toEqual({
      exercise_style: 'european',
      right_type: 'call',
      strike: '100',
      time_to_expiry: '1',
      spot: '100',
      volatility: '0.20',
      risk_free_rate: '0.05',
      dividend_yield: '0',
    });
    request.flush({ model: 'black_scholes', price: '10.450583572185565' });
    await fixture.whenStable();
    expect(root.querySelector('output')?.textContent).toBe('10.450583572185565');
    input('spot', '101.12345678');
    await fixture.whenStable();
    expect(root.querySelector('output')).toBeNull();
    await calculate();
    const next = http.expectOne(endpoint);
    expect(next.request.body.spot).toBe('101.12345678');
    next.flush({ model: 'black_scholes', price: '11' });
  });

  it('cancels an in-flight calculation on editing or resetting the form', async () => {
    await calculate();
    const pending = http.expectOne(endpoint);
    input('strike', '90');
    expect(pending.cancelled).toBe(true);
    expect(fixture.componentInstance.pricing.request().isLoading).toBe(false);
    await fixture.whenStable();
    await calculate();
    const next = http.expectOne(endpoint);
    root.querySelector<HTMLButtonElement>('button[type="button"]')!.click();
    expect(next.cancelled).toBe(true);
    await fixture.whenStable();
    expect(root.querySelector<HTMLInputElement>('#pricing-strike')!.value).toBe('100');
    expect(root.querySelector('output')).toBeNull();
  });

  it('accepts puts, expiry, zero volatility, and negative rates as strings', async () => {
    const right = root.querySelector<HTMLSelectElement>('#pricing-right')!;
    right.value = 'put';
    right.dispatchEvent(new Event('input', { bubbles: true }));
    right.dispatchEvent(new Event('change', { bubbles: true }));
    input('time_to_expiry', '0');
    input('volatility', '0');
    input('risk_free_rate', '-0.05');
    input('dividend_yield', '-0.02');
    await fixture.whenStable();
    await calculate();
    const request = http.expectOne(endpoint);
    expect(request.request.body).toMatchObject({
      right_type: 'put',
      time_to_expiry: '0',
      volatility: '0',
      risk_free_rate: '-0.05',
      dividend_yield: '-0.02',
    });
    request.flush({ model: 'black_scholes', price: '0' });
    await fixture.whenStable();
    expect(root.querySelector('output')?.textContent).toBe('0');
  });

  it.each([
    ['spot', ''],
    ['strike', '0'],
    ['spot', '-1'],
    ['volatility', '-0.1'],
    ['time_to_expiry', '-1'],
    ['spot', 'NaN'],
    ['risk_free_rate', 'Infinity'],
    ['strike', '1e2'],
    ['spot', ' 100'],
    ['spot', '1000000000000'],
    ['spot', '0.123456789'],
  ])('rejects %s=%s before making an HTTP request', async (key, value) => {
    input(key, value);
    await fixture.whenStable();
    await calculate();
    await fixture.whenStable();
    http.expectNone(endpoint);
    expect(root.querySelector('#error-' + key)?.textContent?.trim()).toBeTruthy();
    expect(root.querySelector('#pricing-' + key)?.getAttribute('aria-invalid')).toBe('true');
  });

  it('renders an API error and allows retry without losing the draft', async () => {
    input('risk_free_rate', '-1000');
    await fixture.whenStable();
    await calculate();
    http
      .expectOne(endpoint)
      .flush(
        { error: 'Pricing inputs exceed the model range' },
        { status: 400, statusText: 'Bad Request' },
      );
    await fixture.whenStable();
    expect(root.querySelector('[role="alert"]')?.textContent).toContain('model range');
    expect(root.querySelector('button[type="submit"]')?.textContent).toContain('Retry');
    expect(root.querySelector<HTMLInputElement>('#pricing-risk_free_rate')!.value).toBe('-1000');
    input('risk_free_rate', '0.05');
    await fixture.whenStable();
    await calculate();
    http.expectOne(endpoint).flush({ model: 'black_scholes', price: '10.450583572185565' });
    await fixture.whenStable();
    expect(root.querySelector('[role="alert"]')).toBeNull();
    expect(root.querySelector('output')).not.toBeNull();
  });

  it('cancels the request when its component-scoped store is destroyed', async () => {
    await calculate();
    const pending = http.expectOne(endpoint);
    fixture.destroy();
    expect(pending.cancelled).toBe(true);
  });
});
