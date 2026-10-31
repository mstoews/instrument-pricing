import { ComponentFixture, TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { provideHttpClientTesting, HttpTestingController } from '@angular/common/http/testing';
import { provideRouter, Router } from '@angular/router';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { Login } from './login';
import { pair } from '../../testing/auth-fixtures';
import { authGuard, guestGuard } from '../../core/auth.guards';
import { Component } from '@angular/core';

@Component({ template: 'workspace' })
class Workspace {}

describe('Login', () => {
 let fixture: ComponentFixture<Login>;
 let http: HttpTestingController;
 let root: HTMLElement;
 beforeEach(async () => {
  sessionStorage.clear();
  localStorage.clear();
  TestBed.configureTestingModule({
   imports: [Login],
   providers: [provideHttpClient(), provideHttpClientTesting(), provideRouter([
    { path: 'login', component: Login, canActivate: [guestGuard] },
    { path: '', component: Workspace, canActivate: [authGuard] },
   ])],
  });
  http = TestBed.inject(HttpTestingController);
  fixture = TestBed.createComponent(Login);
  fixture.componentRef.setInput('returnUrl', '/');
  root = fixture.nativeElement;
  await fixture.whenStable();
 });
 afterEach(() => {
  try {
   http.verify();
  } finally {
   TestBed.resetTestingModule();
  }
 });
 const type = (id: string, value: string) => {
  const field = root.querySelector<HTMLInputElement>('#' + id)!;
  field.value = value;
  field.dispatchEvent(new Event('input', { bubbles: true }));
  field.dispatchEvent(new Event('blur'));
 };
 const submit = async () => {
  root.querySelector('form')!.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));
  await fixture.whenStable();
 };

 it('validates before sending anything', async () => {
  await submit();
  http.expectNone('/v1/auth/login');
  expect(root.textContent).toContain('Enter your email address.');
  expect(root.textContent).toContain('Enter your password.');
  type('login-email', 'not-an-email');
  await fixture.whenStable();
  expect(root.textContent).toContain('Enter a valid email address.');
  expect(root.querySelector('#login-email')!.getAttribute('aria-invalid')).toBe('true');
 });

 it('signs in and shows server errors accessibly', async () => {
  type('login-email', ' analyst@example.com ');
  type('login-password', 'wrong password');
  await submit();
  const req = http.expectOne('/v1/auth/login');
  expect(req.request.body).toEqual({ email: 'analyst@example.com', password: 'wrong password' });
  await fixture.whenStable();
  expect(root.querySelector<HTMLButtonElement>('.login-submit')!.disabled).toBe(true);
  expect(root.textContent).toContain('Signing in…');
  req.flush({ error: 'unauthorized' }, { status: 401, statusText: 'Unauthorized' });
  await fixture.whenStable();
  expect(root.querySelector('[role=alert]')!.textContent).toContain('Email or password is incorrect.');
  type('login-password', 'correct password');
  await fixture.whenStable();
  expect(root.querySelector('[role=alert]')).toBeNull(); // editing clears the stale error
  await submit();
  http.expectOne('/v1/auth/login').flush(pair(1));
  await fixture.whenStable();
  expect(TestBed.inject(Router).url).toBe('/');
 });

 it('guards the workspace and the sign-in page', async () => {
  const router = TestBed.inject(Router);
  await router.navigateByUrl('/?tab=curves');
  expect(router.url).toBe('/login?returnUrl=%2F%3Ftab%3Dcurves');
  type('login-email', 'analyst@example.com');
  type('login-password', 'correct password');
  await submit();
  http.expectOne('/v1/auth/login').flush(pair(1));
  await fixture.whenStable();
  await router.navigateByUrl('/login');
  expect(router.url).toBe('/'); // already signed in
 });
});
