import { ApplicationConfig, inject, provideAppInitializer, provideBrowserGlobalErrorListeners } from '@angular/core';
import { provideHttpClient, withInterceptors } from '@angular/common/http';
import { provideRouter, withComponentInputBinding } from '@angular/router';
import { routes } from './app.routes';
import { authInterceptor } from './core/auth.interceptor';
import { AuthStore } from './state/auth.store';
export const appConfig: ApplicationConfig = {
 providers: [
  provideBrowserGlobalErrorListeners(),
  provideHttpClient(withInterceptors([authInterceptor])),
  provideRouter(routes, withComponentInputBinding()),
  // Restore a stored session before the first navigation so guards decide synchronously.
  provideAppInitializer(() => inject(AuthStore).restore()),
 ],
};
