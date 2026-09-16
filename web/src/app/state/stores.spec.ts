import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { provideHttpClientTesting, HttpTestingController } from '@angular/common/http/testing';
import { describe, it, expect, beforeEach, afterEach } from 'vitest';
import { InstrumentStore } from './instrument.store';
import { OptionWorkspaceStore } from './option-workspace.store';
const instrument = (id: string) => ({ id, name: id, instrument_type: 'equity', currency: 'USD' });
describe('market stores', () => {
 let http: HttpTestingController;
 beforeEach(() => { TestBed.configureTestingModule({ providers: [provideHttpClient(), provideHttpClientTesting(), InstrumentStore, OptionWorkspaceStore] }); http = TestBed.inject(HttpTestingController); });
 afterEach(() => { http.verify({ ignoreCancelled: true }); TestBed.resetTestingModule(); });
 it('cancels old searches and recovers after errors', () => {
  const store = TestBed.inject(InstrumentStore);
  store.search('old'); const old = http.expectOne('/v1/instruments?q=old');
  store.search('new'); expect(old.cancelled).toBe(true); expect(store.request().isLoading).toBe(true);
  http.expectOne('/v1/instruments?q=new').flush({}, { status: 500, statusText: 'Failure' });
  expect(store.request().isLoaded).toBe(false); expect(store.request().error).toBeTruthy();
  store.search('AAPL'); http.expectOne('/v1/instruments?q=AAPL').flush([instrument('a')]);
  expect(store.results()[0].id).toBe('a'); expect(store.request().isLoaded).toBe(true); expect(store.request().error).toBeNull();
 });
 it('cancels dependent contracts when the selected instrument changes', () => {
  const store = TestBed.inject(OptionWorkspaceStore);
  store.select(instrument('a'));
  const oldQuote = http.expectOne('/v1/market-data/a/quote');
  http.expectOne('/v1/options/a/expirations').flush(['2027-01-15']);
  const oldChain = http.expectOne('/v1/options/a/chain?expiration=2027-01-15');
  store.select(instrument('b'));
  expect(oldQuote.cancelled).toBe(true); expect(oldChain.cancelled).toBe(true);
  expect(store.chain()).toEqual([]); expect(store.quote()).toBeNull();
  http.expectOne('/v1/market-data/b/quote').flush({}, { status: 404, statusText: 'Not found' });
  http.expectOne('/v1/options/b/expirations').flush([]);
  expect(store.quoteRequest().error).toBeTruthy(); expect(store.expirationsRequest().isLoaded).toBe(true);
  expect(store.chainRequest().isLoading).toBe(false);
 });
 it('cancels old expirations and computes paired strike rows', () => {
  const store = TestBed.inject(OptionWorkspaceStore);
  store.select(instrument('a'));
  http.expectOne('/v1/market-data/a/quote').flush({ instrument_id: 'a', bid: '1', ask: '2' });
  http.expectOne('/v1/options/a/expirations').flush(['2027-01-15','2027-02-19']);
  const old = http.expectOne('/v1/options/a/chain?expiration=2027-01-15');
  store.loadChain('2027-02-19'); expect(old.cancelled).toBe(true);
  http.expectOne('/v1/options/a/chain?expiration=2027-02-19').flush([
   {instrument_id:'c',strike:'200.00000000',right_type:'call',bid:'1',ask:'2'},
   {instrument_id:'p',strike:'200.00000000',right_type:'put',bid:'3',ask:'4'},
  ]);
  expect(store.rows().length).toBe(1); expect(store.rows()[0].put?.bid).toBe('3');
  expect(store.chainRequest().isLoaded).toBe(true);
 });
});
