import { inject, Injectable } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { Quote } from './models';
@Injectable({ providedIn: 'root' })
export class MarketDataService {
 private readonly http = inject(HttpClient);
 quote(id: string) { return this.http.get<Quote>(`/v1/market-data/${encodeURIComponent(id)}/quote`); }
}
