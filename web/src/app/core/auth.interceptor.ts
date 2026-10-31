import { inject } from '@angular/core';
import { HttpErrorResponse, HttpInterceptorFn, HttpRequest } from '@angular/common/http';
import { catchError, switchMap, throwError } from 'rxjs';
import { authEndpoints } from './auth';
import { AuthStore, SessionExpiredError } from '../state/auth.store';

const withToken = (req: HttpRequest<unknown>, token: string) => req.clone({ setHeaders: { Authorization: `Bearer ${token}` } });
const isUnauthorized = (error: unknown) => error instanceof HttpErrorResponse && error.status === 401;

// Attaches the access token to API calls, refreshing it when it is about to
// expire. A 401 triggers one shared refresh and a single retry; if that also
// fails the session ends and the user is sent to sign in.
export const authInterceptor: HttpInterceptorFn = (req, next) => {
 const path = req.url.split('?')[0];
 if (!path.startsWith('/v1/') || authEndpoints.includes(path)) return next(req);
 const auth = inject(AuthStore);
 if (!auth.isSignedIn()) return next(req);
 const send = (token: string) => next(withToken(req, token));
 return auth.validAccessToken().pipe(
  switchMap((token) => send(token).pipe(
   catchError((error) => isUnauthorized(error) ? auth.refresh().pipe(switchMap(send)) : throwError(() => error)),
  )),
  catchError((error) => {
   if (error instanceof SessionExpiredError || isUnauthorized(error)) auth.expire();
   return throwError(() => error);
  }),
 );
};
