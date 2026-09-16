import { inject, Injectable } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { OptionContract } from './models';
@Injectable({ providedIn: 'root' })
export class OptionsService {
 private readonly http = inject(HttpClient);
 expirations(id: string) { return this.http.get<string[]>(`/v1/options/${encodeURIComponent(id)}/expirations`); }
 chain(id: string, expiration: string) { return this.http.get<OptionContract[]>(`/v1/options/${encodeURIComponent(id)}/chain`, { params: { expiration } }); }
}
