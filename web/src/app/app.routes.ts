import { Routes } from '@angular/router';
import { authGuard, guestGuard } from './core/auth.guards';

export const routes: Routes = [
 { path: 'login', canActivate: [guestGuard], title: 'Sign in · Pricing platform', loadComponent: () => import('./features/login/login').then((m) => m.Login) },
 { path: '', canActivate: [authGuard], title: 'Pricing platform', loadComponent: () => import('./features/option-workspace/option-workspace').then((m) => m.OptionWorkspace) },
 { path: '**', redirectTo: '' },
];
