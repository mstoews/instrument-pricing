export interface Instrument { id: string; instrument_type: string; currency: string; name: string }
export interface Quote { instrument_id: string; source_id: string; observed_at: string; received_at: string; bid: string; ask: string }
export interface OptionContract { instrument_id: string; expiration: string; strike: string; right_type: 'call' | 'put'; exercise_style: string; bid: string; ask: string; source_id: string }
export interface RequestState { isLoading: boolean; isLoaded: boolean; error: string | null }
export const idle = (): RequestState => ({ isLoading: false, isLoaded: false, error: null });
export const pending = (): RequestState => ({ isLoading: true, isLoaded: false, error: null });
export const loaded = (): RequestState => ({ isLoading: false, isLoaded: true, error: null });
export function failed(error: unknown): RequestState {
 return { ...idle(), error: error instanceof Error ? error.message : 'Request failed. Please retry.' };
}

export interface EuropeanPriceRequest {
 exercise_style: 'european';
 right_type: 'call' | 'put';
 strike: string;
 time_to_expiry: string;
 spot: string;
 volatility: string;
 risk_free_rate: string;
 dividend_yield: string;
}
export interface EuropeanPriceResult { model: 'black_scholes'; price: string }

export interface Curve { id: string; name: string; currency: string }
export interface CurvePoint { tenor_days: number; zero_rate: string; observed_at: string; source_id: string }
export interface CurveRateSelection { curve: Curve; point: CurvePoint }
