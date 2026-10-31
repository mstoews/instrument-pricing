import { Component } from '@angular/core';
import { provideHttpClient } from '@angular/common/http';
import { provideHttpClientTesting } from '@angular/common/http/testing';
import { provideRouter } from '@angular/router';
import { TokenPair } from '../core/auth';

@Component({ template: '' })
export class Blank {}

export function pair(n: number, accessMs = 15 * 60_000): TokenPair {
 return {
  token_type: 'Bearer', access_token: `ipa_access${n}`, access_expires_at: new Date(Date.now() + accessMs).toISOString(),
  refresh_token: `ipr_refresh${n}`, refresh_expires_at: new Date(Date.now() + 86_400_000).toISOString(),
  user: { id: 'u1', email: 'analyst@example.com', role: 'user' },
 };
}
export const authProviders = () => [provideHttpClient(), provideHttpClientTesting(), provideRouter([{ path: '**', component: Blank }])];
// Lets promise chains (the cross-tab refresh lock, router navigation) settle.
export const tick = () => new Promise((resolve) => setTimeout(resolve));
