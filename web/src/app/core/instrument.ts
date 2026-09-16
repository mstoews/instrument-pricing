import { inject, Injectable } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { Instrument } from './models';
@Injectable({ providedIn: 'root' })
export class InstrumentService {
 private readonly http = inject(HttpClient);
 search(q: string) { return this.http.get<Instrument[]>('/v1/instruments', { params: { q } }); }
 resolve(value: string, venue = 'XNAS', scheme = 'ticker') { return this.http.get<Instrument[]>('/v1/instruments/resolve', { params: { value, venue, scheme } }); }
}
