import { Injectable } from '@angular/core';

// Only the refresh token is persisted; the access token lives in memory.
// "Keep me signed in" uses localStorage (shared by tabs, survives restarts);
// otherwise sessionStorage (this tab only, cleared when it closes).
export const refreshTokenKey = 'pricing.refresh_token';
export interface StoredRefresh { token: string; expiresAt: string; remember: boolean }

function area(remember: boolean): Storage | null {
 try { return remember ? localStorage : sessionStorage; } catch { return null; }
}

@Injectable({ providedIn: 'root' })
export class TokenStorage {
 read(): StoredRefresh | null {
  for (const remember of [true, false]) {
   try {
    const raw = area(remember)?.getItem(refreshTokenKey);
    if (!raw) continue;
    const value = JSON.parse(raw) as StoredRefresh;
    if (typeof value.token === 'string' && Date.parse(value.expiresAt) > Date.now()) return { ...value, remember };
   } catch { /* corrupt or blocked storage: treat as signed out */ }
  }
  return null;
 }
 write(token: string, expiresAt: string, remember: boolean) {
  // Overwrite in place: removing first would tell other tabs we signed out.
  try { area(!remember)?.removeItem(refreshTokenKey); } catch { /* ignore */ }
  try { area(remember)?.setItem(refreshTokenKey, JSON.stringify({ token, expiresAt, remember })); } catch { /* storage blocked: session lasts until reload */ }
 }
 clear() {
  for (const remember of [true, false]) try { area(remember)?.removeItem(refreshTokenKey); } catch { /* ignore */ }
 }
}

// Serializes refreshes across tabs that share localStorage. A refresh token
// is single-use: two tabs spending it concurrently would revoke the login.
export function withRefreshLock<T>(work: () => Promise<T>): Promise<T> {
 const locks = typeof navigator !== 'undefined' ? navigator.locks : undefined;
 return locks ? locks.request('pricing-auth-refresh', work) : work();
}
