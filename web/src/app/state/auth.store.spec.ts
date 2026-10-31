import { TestBed } from '@angular/core/testing';
import { HttpTestingController } from '@angular/common/http/testing';
import { Router } from '@angular/router';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { firstValueFrom } from 'rxjs';
import { tick, authProviders, pair } from '../testing/auth-fixtures';
import { refreshTokenKey } from '../core/token-storage';
import { AuthStore, safeReturnUrl, SessionExpiredError } from './auth.store';

const stored = (area: Storage) => JSON.parse(area.getItem(refreshTokenKey) ?? 'null');

describe('AuthStore', () => {
 let http: HttpTestingController;
 let store: InstanceType<typeof AuthStore>;
 let router: Router;
 beforeEach(() => {
  localStorage.clear();
  sessionStorage.clear();
  TestBed.configureTestingModule({ providers: authProviders() });
  http = TestBed.inject(HttpTestingController);
  store = TestBed.inject(AuthStore);
  router = TestBed.inject(Router);
 });
 afterEach(() => {
  try {
   http.verify();
  } finally {
   TestBed.resetTestingModule();
  }
 });

 it('signs in, keeps the refresh token in tab storage by default and follows a safe return URL', async () => {
  store.login({ email: 'analyst@example.com', password: 'pw', remember: false, returnUrl: '/?x=1' });
  store.login({ email: 'analyst@example.com', password: 'pw', remember: false }); // ignored while pending
  expect(store.request().isLoading).toBe(true);
  const req = http.expectOne('/v1/auth/login');
  expect(req.request.body).toEqual({ email: 'analyst@example.com', password: 'pw' });
  req.flush(pair(1));
  await tick(); // the store schedules navigation
  expect(store.isSignedIn()).toBe(true);
  expect(store.user()?.email).toBe('analyst@example.com');
  expect(stored(sessionStorage).token).toBe('ipr_refresh1');
  expect(localStorage.getItem(refreshTokenKey)).toBeNull();
  expect(router.url).toBe('/?x=1');
 });

 it('persists across restarts only when asked to', () => {
  store.login({ email: 'a@example.com', password: 'pw', remember: true });
  http.expectOne('/v1/auth/login').flush(pair(1));
  expect(stored(localStorage).token).toBe('ipr_refresh1');
  expect(sessionStorage.getItem(refreshTokenKey)).toBeNull();
 });

 it.each([
  [401, 'Email or password is incorrect.'],
  [429, 'Too many failed attempts. Wait 15 minutes, then try again.'],
  [0, 'Cannot reach the server. Check your connection and retry.'],
  [500, 'Sign-in failed. Please retry.'],
 ])('explains a %i login failure', (status, message) => {
  store.login({ email: 'a@example.com', password: 'pw', remember: false });
  const req = http.expectOne('/v1/auth/login');
  if (status === 0) req.error(new ProgressEvent('error'));
  else req.flush({ error: 'x' }, { status, statusText: 'x' });
  expect(store.request().error).toBe(message);
  expect(store.isSignedIn()).toBe(false);
 });

 it('restores a stored session through one refresh', async () => {
  localStorage.setItem(refreshTokenKey, JSON.stringify({ token: 'ipr_old', expiresAt: pair(0).refresh_expires_at, remember: true }));
  const restoring = store.restore();
  await tick();
  const req = http.expectOne('/v1/auth/refresh');
  expect(req.request.body).toEqual({ refresh_token: 'ipr_old' });
  req.flush(pair(2));
  await restoring;
  expect(store.isSignedIn()).toBe(true);
  expect(stored(localStorage).token).toBe('ipr_refresh2'); // rotated, same storage area
 });

 it('drops a rejected stored session but keeps it through a network outage', async () => {
  const token = JSON.stringify({ token: 'ipr_old', expiresAt: pair(0).refresh_expires_at, remember: true });
  localStorage.setItem(refreshTokenKey, token);
  let restoring = store.restore();
  await tick();
  http.expectOne('/v1/auth/refresh').error(new ProgressEvent('error'));
  await restoring;
  expect(store.status()).toBe('signedOut');
  expect(localStorage.getItem(refreshTokenKey)).toBe(token);
  restoring = store.restore();
  await tick();
  http.expectOne('/v1/auth/refresh').flush({ error: 'unauthorized' }, { status: 401, statusText: 'Unauthorized' });
  await restoring;
  expect(localStorage.getItem(refreshTokenKey)).toBeNull();
 });

 it('ignores expired or corrupt stored tokens without a request', async () => {
  localStorage.setItem(refreshTokenKey, JSON.stringify({ token: 'ipr_old', expiresAt: '2000-01-01T00:00:00Z', remember: true }));
  sessionStorage.setItem(refreshTokenKey, '{not json');
  await store.restore();
  http.expectNone('/v1/auth/refresh');
  expect(store.status()).toBe('signedOut');
 });

 it('shares one refresh between concurrent callers', async () => {
  store.login({ email: 'a@example.com', password: 'pw', remember: false });
  http.expectOne('/v1/auth/login').flush(pair(1, 10_000)); // inside the expiry skew
  const a = firstValueFrom(store.validAccessToken());
  const b = firstValueFrom(store.validAccessToken());
  await tick();
  http.expectOne('/v1/auth/refresh').flush(pair(2));
  expect(await a).toBe('ipa_access2');
  expect(await b).toBe('ipa_access2');
  expect(await firstValueFrom(store.validAccessToken())).toBe('ipa_access2');
  http.expectNone('/v1/auth/refresh');
 });

 it('reports a rejected refresh as an expired session', async () => {
  store.login({ email: 'a@example.com', password: 'pw', remember: false });
  http.expectOne('/v1/auth/login').flush(pair(1));
  const result = firstValueFrom(store.refresh()).catch((e) => e);
  await tick();
  http.expectOne('/v1/auth/refresh').flush({ error: 'unauthorized' }, { status: 401, statusText: 'Unauthorized' });
  expect(await result).toBeInstanceOf(SessionExpiredError);
 });

 it('signs out locally even when the server is unreachable', async () => {
  store.login({ email: 'a@example.com', password: 'pw', remember: true });
  http.expectOne('/v1/auth/login').flush(pair(1));
  const done = store.logout();
  expect(store.isSignedIn()).toBe(false);
  expect(localStorage.getItem(refreshTokenKey)).toBeNull();
  const req = http.expectOne('/v1/auth/logout');
  expect(req.request.body).toEqual({ refresh_token: 'ipr_refresh1' });
  expect(req.request.headers.has('Authorization')).toBe(false);
  req.error(new ProgressEvent('error'));
  await done;
 });

 it('rotates the stored token without a removal other tabs would read as sign-out', async () => {
  const removed: string[] = [];
  const original = Storage.prototype.removeItem;
  Storage.prototype.removeItem = function (this: Storage, key: string) {
   if (this === localStorage) removed.push(key);
   return original.call(this, key);
  };
  try {
   store.login({ email: 'a@example.com', password: 'pw', remember: true });
   http.expectOne('/v1/auth/login').flush(pair(1));
   const refreshed = firstValueFrom(store.refresh());
   await tick();
   http.expectOne('/v1/auth/refresh').flush(pair(2));
   await refreshed;
  } finally {
   Storage.prototype.removeItem = original;
  }
  expect(stored(localStorage).token).toBe('ipr_refresh2');
  expect(removed).toEqual([]);
 });

 it('follows a sign-out in another tab', async () => {
  store.login({ email: 'a@example.com', password: 'pw', remember: true });
  http.expectOne('/v1/auth/login').flush(pair(1));
  localStorage.removeItem(refreshTokenKey);
  window.dispatchEvent(new StorageEvent('storage', { key: refreshTokenKey, newValue: null }));
  expect(store.isSignedIn()).toBe(false);
  expect(store.notice()).toBe('You signed out in another tab.');
 });

 it('only returns to same-origin paths', () => {
  expect(safeReturnUrl('/curves?c=USD')).toBe('/curves?c=USD');
  for (const bad of [null, '', 'https://evil.example', '//evil.example', '/login?returnUrl=/']) expect(safeReturnUrl(bad)).toBe('/');
 });
});
