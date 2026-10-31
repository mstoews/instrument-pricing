import { computed, DestroyRef, inject } from '@angular/core';
import { HttpErrorResponse } from '@angular/common/http';
import { Router } from '@angular/router';
import { patchState, signalStore, withComputed, withHooks, withMethods, withState } from '@ngrx/signals';
import { rxMethod } from '@ngrx/signals/rxjs-interop';
import { tapResponse } from '@ngrx/operators';
import { defer, exhaustMap, finalize, firstValueFrom, from, Observable, of, pipe, shareReplay } from 'rxjs';
import { AuthService, AuthUser, Credentials, TokenPair } from '../core/auth';
import { refreshTokenKey, TokenStorage, withRefreshLock } from '../core/token-storage';
import { RequestState, idle, pending, loaded } from '../core/models';

// Thrown when the server rejects the refresh token: the user must sign in again.
export class SessionExpiredError extends Error {
 constructor() { super('Your session has expired. Sign in again.'); }
}

export interface LoginRequest extends Credentials { remember: boolean; returnUrl?: string | null }
type Status = 'restoring' | 'signedOut' | 'signedIn';
interface AuthState {
 status: Status;
 user: AuthUser | null;
 accessToken: string | null;
 accessExpiresAt: number;
 request: RequestState;
 notice: string | null;
}
const signedOut = { status: 'signedOut' as Status, user: null, accessToken: null, accessExpiresAt: 0 };
// Refresh slightly early so a token does not expire in flight.
const expirySkewMs = 30_000;

function loginError(error: unknown): RequestState {
 let message = 'Sign-in failed. Please retry.';
 if (error instanceof HttpErrorResponse) {
  if (error.status === 0) message = 'Cannot reach the server. Check your connection and retry.';
  else if (error.status === 401 || error.status === 400) message = 'Email or password is incorrect.';
  else if (error.status === 429) message = 'Too many failed attempts. Wait 15 minutes, then try again.';
 }
 return { ...idle(), error: message };
}

// Only same-origin paths: never redirect to another site after sign-in.
export function safeReturnUrl(url: string | null | undefined): string {
 return url && url.startsWith('/') && !url.startsWith('//') && !url.startsWith('/login') ? url : '/';
}

export const AuthStore = signalStore(
 { providedIn: 'root' },
 withState<AuthState>({ ...signedOut, status: 'restoring', request: idle(), notice: null }),
 withComputed((store) => ({ isSignedIn: computed(() => store.status() === 'signedIn') })),
 withMethods((store, api = inject(AuthService), storage = inject(TokenStorage), router = inject(Router)) => {
  let inFlight: Observable<string> | null = null;
  const accept = (pair: TokenPair, remember: boolean) => {
   storage.write(pair.refresh_token, pair.refresh_expires_at, remember);
   patchState(store, {
    status: 'signedIn', user: pair.user, accessToken: pair.access_token,
    accessExpiresAt: Date.parse(pair.access_expires_at), notice: null,
   });
  };
  const forget = (notice: string | null = null) => {
   storage.clear();
   patchState(store, { ...signedOut, notice });
  };
  const refreshOnce = () => withRefreshLock(async () => {
   // Read inside the lock: another tab may have rotated the token meanwhile.
   const stored = storage.read();
   if (!stored) throw new SessionExpiredError();
   try {
    const pair = await firstValueFrom(api.refresh(stored.token));
    accept(pair, stored.remember);
    return pair.access_token;
   } catch (error) {
    // A network failure keeps the session; a rejected token ends it.
    if (error instanceof HttpErrorResponse && error.status !== 0 && error.status < 500) throw new SessionExpiredError();
    throw error;
   }
  });
  // All concurrent callers share one refresh request.
  const refresh = (): Observable<string> =>
   inFlight ??= defer(() => from(refreshOnce())).pipe(finalize(() => (inFlight = null)), shareReplay({ bufferSize: 1, refCount: false }));

  return {
   refresh,
   validAccessToken(): Observable<string> {
    const token = store.accessToken();
    return token && store.accessExpiresAt() - expirySkewMs > Date.now() ? of(token) : refresh();
   },
   async restore(): Promise<void> {
    if (!storage.read()) { patchState(store, signedOut); return; }
    try {
     await firstValueFrom(refresh());
    } catch (error) {
     // Keep a stored token through a network outage so the next load can retry.
     if (error instanceof SessionExpiredError) forget();
     else patchState(store, signedOut);
    }
   },
   login: rxMethod<LoginRequest>(pipe(
    // Ignore repeat submits while a sign-in is pending.
    exhaustMap(({ email, password, remember, returnUrl }) => {
     patchState(store, { request: pending(), notice: null });
     return api.login({ email, password }).pipe(tapResponse({
      next: (pair) => {
       accept(pair, remember);
       patchState(store, { request: loaded() });
       void router.navigateByUrl(safeReturnUrl(returnUrl));
      },
      error: (error) => patchState(store, { request: loginError(error) }),
     }));
    }),
   )),
   expire() {
    // Several in-flight requests can fail together; redirect once.
    if (store.status() !== 'signedIn' && router.url.startsWith('/login')) return;
    forget('Your session has expired. Sign in again.');
    void router.navigate(['/login'], { queryParams: { returnUrl: safeReturnUrl(router.url) } });
   },
   async logout() {
    const token = storage.read()?.token;
    forget();
    void router.navigate(['/login']);
    // Best effort: local tokens are already gone if the server is unreachable.
    if (token) try { await firstValueFrom(api.logout(token)); } catch { /* ignore */ }
   },
   clearError() { patchState(store, { request: idle() }); },
   _signedOutElsewhere() {
    if (store.status() !== 'signedIn' || storage.read()) return;
    patchState(store, { ...signedOut, notice: 'You signed out in another tab.' });
    void router.navigate(['/login']);
   },
  };
 }),
 withHooks({
  onInit(store, destroyRef = inject(DestroyRef)) {
   // Another tab signing out removes the shared refresh token.
   if (typeof window === 'undefined') return;
   const onStorage = (event: StorageEvent) => { if (event.key === refreshTokenKey && event.newValue === null) store._signedOutElsewhere(); };
   window.addEventListener('storage', onStorage);
   destroyRef.onDestroy(() => window.removeEventListener('storage', onStorage));
  },
 }),
);
