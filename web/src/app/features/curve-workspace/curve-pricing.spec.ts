import { ComponentFixture, TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { provideHttpClientTesting, HttpTestingController } from '@angular/common/http/testing';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { EuropeanPricer } from '../european-pricer/european-pricer';
import { CurveWorkspace } from './curve-workspace';
import { By } from '@angular/platform-browser';

const curve = { id: 'usd', name: 'Sample zero curve', currency: 'USD' };
const point = {
  tenor_days: 365,
  zero_rate: '-0.01234567',
  source_id: 'sample',
  observed_at: '2026-09-15T12:00:00Z',
};
describe('curve to scenario workflow', () => {
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
  const button = (text: string) =>
    [...root.querySelectorAll<HTMLButtonElement>('button')].find((b) =>
      b.textContent?.trim().startsWith(text),
    )!;
  const edit = (id: string, value: string) => {
    const field = root.querySelector<HTMLInputElement>('#' + id)!;
    field.value = value;
    field.dispatchEvent(new Event('input', { bubbles: true }));
  };
  const load = async () => {
    root.querySelector('app-curve-workspace details')!.setAttribute('open', '');
    root
      .querySelector('app-curve-workspace form')!
      .dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));
    await Promise.resolve();
  };
  const select = async (points = [point]) => {
    await load();
    http.expectOne('/v1/curves?currency=USD').flush([curve]);
    await fixture.whenStable();
    button('Sample zero curve').click();
    http.expectOne('/v1/curves/usd/points').flush(points);
    await fixture.whenStable();
  };
  const price = async () => {
    root
      .querySelector('form')!
      .dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));
    await Promise.resolve();
  };

  it('loads on demand and copies an exact negative rate with provenance, cancelling prior pricing', async () => {
    http.expectNone((request) => request.url.startsWith('/v1/curves'));
    await select();
    expect(root.textContent).toContain('-0.01234567');
    expect(root.textContent).toContain('no interpolation');
    await price();
    const pending = http.expectOne('/v1/pricing/european');
    button('Use rate').click();
    expect(pending.cancelled).toBe(true);
    await fixture.whenStable();
    expect(root.querySelector<HTMLInputElement>('#pricing-risk_free_rate')!.value).toBe(
      '-0.01234567',
    );
    expect(root.textContent).toContain('Copied USD 365-day rate from Sample zero curve');
    await price();
    const request = http.expectOne('/v1/pricing/european');
    expect(request.request.body.risk_free_rate).toBe('-0.01234567');
    request.flush({ model: 'black_scholes', price: '7.123456789' });
    await fixture.whenStable();
    button('Refresh points').click();
    http.expectOne('/v1/curves/usd/points').flush([{ ...point, zero_rate: '0.03' }]);
    await fixture.whenStable();
    expect(root.querySelector<HTMLInputElement>('#pricing-risk_free_rate')!.value).toBe(
      '-0.01234567',
    );
    expect(root.querySelector('output')?.textContent).toBe('7.123456789');
    edit('pricing-risk_free_rate', '0');
    await fixture.whenStable();
    expect(root.textContent).not.toContain('Copied USD');
    expect(root.querySelector('output')).toBeNull();
  });
  it('shows empty lists, list errors, point errors, and allows retry', async () => {
    await load();
    http.expectOne('/v1/curves?currency=USD').flush([]);
    await fixture.whenStable();
    expect(root.textContent).toContain('No curves available');
    await load();
    http.expectOne('/v1/curves?currency=USD').flush({}, { status: 500, statusText: 'Failure' });
    await fixture.whenStable();
    expect(root.textContent).toContain('Unable to load curves');
    await load();
    http.expectOne('/v1/curves?currency=USD').flush([curve]);
    await fixture.whenStable();
    button('Sample zero curve').click();
    http.expectOne('/v1/curves/usd/points').flush({}, { status: 500, statusText: 'Failure' });
    await fixture.whenStable();
    expect(root.textContent).toContain('Unable to load curve points');
    button('Retry points').click();
    http.expectOne('/v1/curves/usd/points').flush([]);
    await fixture.whenStable();
    expect(root.textContent).toContain('No points in this curve snapshot');
  });
  it.each(['', 'US', 'USDD', '12$'])(
    'rejects invalid currency %s before HTTP',
    async (currency) => {
      edit('curve-currency', currency);
      await fixture.whenStable();
      await load();
      await fixture.whenStable();
      http.expectNone((request) => request.url === '/v1/curves');
      expect(root.querySelector('#currency-error')?.textContent?.trim()).toBeTruthy();
    },
  );
  it('locks linked contracts to their currency and cancels old currency reads on context change', async () => {
    fixture.componentRef.setInput('contract', {
      instrument_id: 'call',
      strike: '100',
      right_type: 'call',
      exercise_style: 'european',
      expiration: '2027-01-15',
    });
    fixture.componentRef.setInput('instrument', {
      id: 'a',
      name: 'A',
      currency: 'EUR',
      instrument_type: 'equity',
    });
    await fixture.whenStable();
    expect(root.querySelector<HTMLInputElement>('#curve-currency')!.readOnly).toBe(true);
    expect(root.querySelector<HTMLInputElement>('#curve-currency')!.value).toBe('EUR');
    await load();
    const pending = http.expectOne('/v1/curves?currency=EUR');
    fixture.componentRef.setInput('instrument', {
      id: 'b',
      name: 'B',
      currency: 'USD',
      instrument_type: 'equity',
    });
    await fixture.whenStable();
    expect(pending.cancelled).toBe(true);
    fixture.componentInstance.useCurveRate({ curve: { ...curve, currency: 'EUR' }, point });
    expect(fixture.componentInstance.model().risk_free_rate).toBe('');
  });
  it('does not silently round unsupported rates and accepts zero', async () => {
    await select([{ ...point, zero_rate: '0.123456789' }]);
    expect(button('Use rate').disabled).toBe(true);
    expect(root.textContent).toContain('Outside pricing precision');
    button('Refresh points').click();
    http.expectOne('/v1/curves/usd/points').flush([{ ...point, zero_rate: '0' }]);
    await fixture.whenStable();
    button('Use rate').click();
    await fixture.whenStable();
    expect(root.querySelector<HTMLInputElement>('#pricing-risk_free_rate')!.value).toBe('0');
    fixture.componentInstance.reset();
    await fixture.whenStable();
    expect(root.textContent).not.toContain('Copied USD');
  });
  it('rejects points outside the current snapshot and cancels reads on destruction', async () => {
    await select();
    const component = fixture.debugElement.query(By.directive(CurveWorkspace))
      .componentInstance as CurveWorkspace;
    component.apply({ ...point, zero_rate: '0.9' });
    expect(fixture.componentInstance.model().risk_free_rate).toBe('0.05');
    button('Refresh points').click();
    const pending = http.expectOne('/v1/curves/usd/points');
    fixture.destroy();
    expect(pending.cancelled).toBe(true);
  });
});
