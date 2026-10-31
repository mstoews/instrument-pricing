import { ChangeDetectionStrategy, Component, inject, input, signal } from '@angular/core';
import { email, form, FormField, maxLength, required, submit } from '@angular/forms/signals';
import { AuthStore } from '../../state/auth.store';

@Component({
 selector: 'app-login',
 imports: [FormField],
 templateUrl: './login.html',
 styleUrl: './login.css',
 changeDetection: ChangeDetectionStrategy.OnPush,
})
export class Login {
 // Bound from the ?returnUrl= query parameter; the store only follows same-origin paths.
 readonly returnUrl = input<string | null>(null);
 readonly auth = inject(AuthStore);
 readonly model = signal({ email: '', password: '', remember: false });
 readonly loginForm = form(this.model, (path) => {
  required(path.email, { message: 'Enter your email address.' });
  email(path.email, { message: 'Enter a valid email address.' });
  maxLength(path.email, 254, { message: 'Email addresses are at most 254 characters.' });
  required(path.password, { message: 'Enter your password.' });
  maxLength(path.password, 128, { message: 'Passwords are at most 128 characters.' });
 });

 signIn() {
  if (this.auth.request().isLoading) return;
  return submit(this.loginForm, async () => {
   const { email, password, remember } = this.model();
   this.auth.login({ email: email.trim(), password, remember, returnUrl: this.returnUrl() });
  });
 }
 edited() { if (this.auth.request().error) this.auth.clearError(); }
}
