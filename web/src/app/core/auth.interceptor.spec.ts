import { TestBed } from '@angular/core/testing';
import { HttpClient, provideHttpClient, withInterceptors } from '@angular/common/http';
import { provideHttpClientTesting, HttpTestingController } from '@angular/common/http/testing';
import { provideRouter, Router } from '@angular/router';
import { Component } from '@angular/core';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { firstValueFrom } from 'rxjs';
import { authInterceptor } from './auth.interceptor';
import { AuthStore } from '../state/auth.store';
import { tick, pair } from '../testing/auth-fixtures';

@Component({ template: '' })
class Blank {}
const unauthorized = { status: 401, statusText: 'Unauthorized' };

describe('authInterceptor', () => {
 let http: HttpTestingController;
 let client: HttpClient;
 let store: InstanceType<typeof AuthStore>;
 const signIn = (accessMs?: number) => {
  store.login({ email: 'a@example.com', password: 'pw', remember: false });
  http.expectOne('/v1/auth/login').flush(pair(1, accessMs));
 };
 beforeEach(() => {
  sessionStorage.clear();
  localStorage.clear();
  TestBed.configureTestingModule({
   providers: [provideHttpClient(withInterceptors([authInterceptor])), provideHttpClientTesting(), provideRouter([{ path: '**', component: Blank }])],
  });
  http = TestBed.inject(HttpTestingController);
  client = TestBed.inject(HttpClient);
  store = TestBed.inject(AuthStore);
 });
 afterEach(() => {
  try {
   http.verify();
  } finally {
   TestBed.resetTestingModule();
  }
 });

 it('attaches the access token to API calls only', () => {
  signIn();
  client.get('/v1/instruments').subscribe();
  client.get('/assets/logo.svg').subscribe();
  client.post('/v1/auth/refresh', {}).subscribe();
  expect(http.expectOne('/v1/instruments').request.headers.get('Authorization')).toBe('Bearer ipa_access1');
  expect(http.expectOne('/assets/logo.svg').request.headers.has('Authorization')).toBe(false);
  expect(http.expectOne('/v1/auth/refresh').request.headers.has('Authorization')).toBe(false);
 });

 it('sends signed-out requests untouched', () => {
  client.get('/v1/instruments').subscribe({ error: () => undefined });
  const req = http.expectOne('/v1/instruments');
  expect(req.request.headers.has('Authorization')).toBe(false);
  req.flush({}, unauthorized);
 });

 it('refreshes an about-to-expire token before sending', async () => {
  signIn(5_000);
  client.get('/v1/curves').subscribe();
  await tick();
  http.expectOne('/v1/auth/refresh').flush(pair(2));
  await tick();
  expect(http.expectOne('/v1/curves').request.headers.get('Authorization')).toBe('Bearer ipa_access2');
 });

 it('retries concurrent 401s after one shared refresh', async () => {
  signIn();
  const a = firstValueFrom(client.get('/v1/a'));
  const b = firstValueFrom(client.get('/v1/b'));
  http.expectOne('/v1/a').flush({}, unauthorized);
  http.expectOne('/v1/b').flush({}, unauthorized);
  await tick();
  http.expectOne('/v1/auth/refresh').flush(pair(2));
  await tick();
  for (const path of ['/v1/a', '/v1/b']) {
   const retry = http.expectOne(path);
   expect(retry.request.headers.get('Authorization')).toBe('Bearer ipa_access2');
   retry.flush({ ok: path });
  }
  expect(await a).toEqual({ ok: '/v1/a' });
  expect(await b).toEqual({ ok: '/v1/b' });
 });

 it('ends the session and returns to sign-in when the refresh is rejected', async () => {
  signIn();
  const router = TestBed.inject(Router);
  await router.navigateByUrl('/workspace?tab=curves');
  const result = firstValueFrom(client.get('/v1/a')).catch((e) => e);
  http.expectOne('/v1/a').flush({}, unauthorized);
  await tick();
  http.expectOne('/v1/auth/refresh').flush({ error: 'unauthorized' }, unauthorized);
  expect((await result).message).toContain('session has expired');
  await tick();
  expect(store.isSignedIn()).toBe(false);
  expect(store.notice()).toBe('Your session has expired. Sign in again.');
  expect(router.url).toBe('/login?returnUrl=%2Fworkspace%3Ftab%3Dcurves');
 });

 it('does not retry other failures', async () => {
  signIn();
  const result = firstValueFrom(client.get('/v1/a')).catch((e) => e.status);
  http.expectOne('/v1/a').flush({}, { status: 403, statusText: 'Forbidden' });
  expect(await result).toBe(403);
  http.expectNone('/v1/auth/refresh');
  expect(store.isSignedIn()).toBe(true);
 });
});
