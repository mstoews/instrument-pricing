import { inject, Injectable } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { Curve, CurvePoint } from './models';

@Injectable({ providedIn: 'root' })
export class CurvesService {
  private readonly http = inject(HttpClient);
  curves(currency: string) {
    return this.http.get<Curve[]>('/v1/curves', { params: { currency } });
  }
  points(id: string) {
    return this.http.get<CurvePoint[]>(`/v1/curves/${encodeURIComponent(id)}/points`);
  }
}

export function isPricingRate(value: string): boolean {
  return /^-?[0-9]{1,12}(\.[0-9]{1,8})?$/.test(value);
}
