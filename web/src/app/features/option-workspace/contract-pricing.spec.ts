import { ComponentFixture, TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { provideHttpClientTesting, HttpTestingController } from '@angular/common/http/testing';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { OptionWorkspace } from './option-workspace';
import { OptionContract } from '../../core/models';

const instrument = {
  id: 'underlying',
  name: 'Sample equity',
  instrument_type: 'equity',
  currency: 'USD',
};
const endpoint = '/v1/pricing/european';
const contract = (
  id: string,
  right_type: 'call' | 'put',
  exercise_style = 'european',
): OptionContract => ({
  instrument_id: id,
  expiration: '2027-01-15',
  strike: '200.12345678',
  right_type,
  exercise_style,
  bid: '',
  ask: '',
  source_id: '',
});

describe('option chain to pricing workflow', () => {
  let fixture: ComponentFixture<OptionWorkspace>;
  let http: HttpTestingController;
  let root: HTMLElement;
  beforeEach(async () => {
    TestBed.configureTestingModule({
      imports: [OptionWorkspace],
      providers: [provideHttpClient(), provideHttpClientTesting()],
    });
    http = TestBed.inject(HttpTestingController);
    fixture = TestBed.createComponent(OptionWorkspace);
    root = fixture.nativeElement;
    await fixture.whenStable();
    http.expectOne('/v1/instruments?q=AAPL').flush([instrument]);
    fixture.componentInstance.select(instrument);
    http.expectOne('/v1/market-data/underlying/quote').flush({
      instrument_id: 'underlying',
      bid: '201.12345678',
      ask: '202.25000000',
      source_id: 'sample',
      observed_at: '2026-09-16T12:00:00Z',
      received_at: '2026-09-16T12:00:01Z',
    });
    http.expectOne('/v1/options/underlying/expirations').flush(['2027-01-15', '2027-02-19']);
    http
      .expectOne('/v1/options/underlying/chain?expiration=2027-01-15')
      .flush([
        contract('call', 'call'),
        contract('put', 'put'),
        { ...contract('american', 'call', 'american'), strike: '300' },
      ]);
    await fixture.whenStable();
  });
  afterEach(() => {
    http.verify();
    TestBed.resetTestingModule();
  });
  const button = (prefix: string) =>
    [...root.querySelectorAll<HTMLButtonElement>('button')].find((b) =>
      b.textContent?.trim().startsWith(prefix),
    )!;
  const edit = (id: string, value: string) => {
    const field = root.querySelector<HTMLInputElement>('#' + id)!;
    field.value = value;
    field.dispatchEvent(new Event('input', { bubbles: true }));
  };
  const calculate = async () => {
    root
      .querySelector('app-european-pricer form')!
      .dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));
    await Promise.resolve();
  };
  const fillMarketInputs = () => {
    for (const [key, value] of Object.entries({
      spot: '201.12345678',
      time_to_expiry: '1',
      volatility: '0.20',
      risk_free_rate: '0.05',
      dividend_yield: '0',
    }))
      edit('pricing-' + key, value);
  };

  it('copies a put exactly and requires explicit quote, time, and market assumptions', async () => {
    button('Use put').click();
    await fixture.whenStable();
    expect(document.activeElement?.id).toBe('pricing-title');
    expect(root.querySelector<HTMLInputElement>('#pricing-strike')!.value).toBe('200.12345678');
    expect(root.querySelector<HTMLSelectElement>('#pricing-right')!.value).toBe('put');
    for (const key of ['spot', 'time_to_expiry', 'volatility', 'risk_free_rate', 'dividend_yield'])
      expect(root.querySelector<HTMLInputElement>('#pricing-' + key)!.value).toBe('');
    await calculate();
    await fixture.whenStable();
    http.expectNone(endpoint);
    button('Use ask').click();
    edit('valuation-date', '2026-01-15');
    await fixture.whenStable();
    button('Use ACT/365F').click();
    edit('pricing-volatility', '0.20');
    edit('pricing-risk_free_rate', '-0.05');
    edit('pricing-dividend_yield', '0');
    await fixture.whenStable();
    expect(root.textContent).toContain('Copied ask from sample');
    await calculate();
    const request = http.expectOne(endpoint);
    expect(request.request.body).toEqual({
      exercise_style: 'european',
      right_type: 'put',
      strike: '200.12345678',
      spot: '202.25000000',
      time_to_expiry: '1.00000000',
      volatility: '0.20',
      risk_free_rate: '-0.05',
      dividend_yield: '0',
    });
    request.flush({ model: 'black_scholes', price: '20.12345678' });
    await fixture.whenStable();
    expect(root.querySelector('output')?.textContent).toBe('20.12345678');
    edit('valuation-date', '2027-01-16');
    await fixture.whenStable();
    expect(root.querySelector('output')).toBeNull();
    expect(root.querySelector<HTMLInputElement>('#pricing-time_to_expiry')!.value).toBe('');
    expect(button('Use ACT/365F').disabled).toBe(true);
    expect(root.querySelector('#date-error')?.textContent).toContain('on or before expiration');
    edit('valuation-date', '2027-01-15');
    await fixture.whenStable();
    button('Use ACT/365F').click();
    await fixture.whenStable();
    expect(root.querySelector<HTMLInputElement>('#pricing-time_to_expiry')!.value).toBe(
      '0.00000000',
    );
  });

  it.each(['expiry', 'refresh', 'instrument', 'contract', 'same contract', 'sample'])(
    'cancels pricing and clears linked state on %s changes',
    async (action) => {
      button('Use call').click();
      await fixture.whenStable();
      fillMarketInputs();
      await fixture.whenStable();
      await calculate();
      const pending = http.expectOne(endpoint);
      if (action === 'expiry') {
        const expiry = root.querySelector<HTMLSelectElement>('#expiry')!;
        expiry.value = '2027-02-19';
        expiry.dispatchEvent(new Event('change', { bubbles: true }));
        http.expectOne('/v1/options/underlying/chain?expiration=2027-02-19').flush([]);
      } else if (action === 'refresh' || action === 'instrument') {
        const id = action === 'refresh' ? 'underlying' : 'second';
        if (action === 'refresh') button('Refresh').click();
        else fixture.componentInstance.select({ ...instrument, id });
        http
          .expectOne('/v1/market-data/' + id + '/quote')
          .flush({}, { status: 404, statusText: 'Not Found' });
        http.expectOne('/v1/options/' + id + '/expirations').flush([]);
      } else if (action === 'contract') button('Use put').click();
      else if (action === 'same contract') button('Use call').click();
      else button('Use sample scenario').click();
      await fixture.whenStable();
      expect(pending.cancelled).toBe(true);
      expect(root.querySelector('output')).toBeNull();
      const linked = action === 'contract' || action === 'same contract';
      if (linked) expect(root.querySelector('#valuation-date')).not.toBeNull();
      else {
        expect(root.querySelector('#valuation-date')).toBeNull();
        expect(fixture.componentInstance.workspace.selectedContract()).toBeNull();
      }
      expect(root.querySelector<HTMLInputElement>('#pricing-spot')!.value).toBe(
        linked ? '' : '100',
      );
    },
  );

  it('disables American contracts and rejects unknown selections in the store', async () => {
    const american = root.querySelector<HTMLButtonElement>(
      '[aria-label="Use call contract at strike 300"]',
    )!;
    expect(american.disabled).toBe(true);
    fixture.componentInstance.workspace.selectContract('american');
    fixture.componentInstance.workspace.selectContract('missing');
    expect(fixture.componentInstance.workspace.selectedContract()).toBeNull();
  });

  it('allows manual spot when the underlying quote is absent', async () => {
    fixture.componentInstance.refresh();
    http
      .expectOne('/v1/market-data/underlying/quote')
      .flush({}, { status: 404, statusText: 'Not Found' });
    http.expectOne('/v1/options/underlying/expirations').flush(['2027-01-15']);
    http
      .expectOne('/v1/options/underlying/chain?expiration=2027-01-15')
      .flush([contract('call', 'call')]);
    await fixture.whenStable();
    button('Use call').click();
    await fixture.whenStable();
    expect(root.textContent).toContain('Underlying quote unavailable');
    expect(button('Use bid')).toBeUndefined();
    fillMarketInputs();
    await fixture.whenStable();
    await calculate();
    http.expectOne(endpoint).flush({ model: 'black_scholes', price: '10' });
    await fixture.whenStable();
    expect(root.querySelector('output')?.textContent).toBe('10');
  });
});
