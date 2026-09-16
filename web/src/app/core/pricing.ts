import { inject, Injectable } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { EuropeanPriceRequest, EuropeanPriceResult } from './models';

@Injectable({ providedIn: 'root' })
export class PricingService {
  private readonly http = inject(HttpClient);
  european(scenario: EuropeanPriceRequest) {
    return this.http.post<EuropeanPriceResult>('/v1/pricing/european', scenario);
  }
}
