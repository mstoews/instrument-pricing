import { inject, Injectable } from '@angular/core';
import { HttpClient } from '@angular/common/http';

export interface AuthUser { id: string; email: string; role: 'user' | 'admin' }
export interface Credentials { email: string; password: string }
export interface TokenPair {
 token_type: 'Bearer';
 access_token: string;
 access_expires_at: string;
 refresh_token: string;
 refresh_expires_at: string;
 user: AuthUser;
}

// Endpoints the auth interceptor must not decorate or retry.
export const authEndpoints = ['/v1/auth/login', '/v1/auth/refresh', '/v1/auth/register', '/v1/auth/logout'];

@Injectable({ providedIn: 'root' })
export class AuthService {
 private readonly http = inject(HttpClient);
 login(credentials: Credentials) { return this.http.post<TokenPair>('/v1/auth/login', credentials); }
 refresh(refreshToken: string) { return this.http.post<TokenPair>('/v1/auth/refresh', { refresh_token: refreshToken }); }
 // Takes the refresh token: the access token may already be expired or rotated by another tab.
 logout(refreshToken: string) { return this.http.post<{ status: string }>('/v1/auth/logout', { refresh_token: refreshToken }); }
}
