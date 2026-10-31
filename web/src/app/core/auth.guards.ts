import { inject } from '@angular/core';
import { CanActivateFn, Router } from '@angular/router';
import { AuthStore } from '../state/auth.store';

// The app initializer restores any stored session before the first navigation,
// so these guards can decide synchronously.
export const authGuard: CanActivateFn = (_route, state) =>
 inject(AuthStore).isSignedIn() || inject(Router).createUrlTree(['/login'], { queryParams: { returnUrl: state.url } });

export const guestGuard: CanActivateFn = () => !inject(AuthStore).isSignedIn() || inject(Router).createUrlTree(['/']);
