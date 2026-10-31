package domain

import (
	"context"
	"time"
)

// Identifier is an instrument identifier; scheme examples: ticker, isin, cusip, figi.
type Identifier struct {
	Scheme string `json:"scheme"`
	Value  string `json:"value"`
	Venue  string `json:"venue"`
}

type NewEquity struct {
	Name        string       `json:"name"`
	Currency    string       `json:"currency"`
	Exchange    string       `json:"exchange"`
	Country     string       `json:"country"`
	Sector      string       `json:"sector"`
	Identifiers []Identifier `json:"identifiers"`
}

// BondCall redeems the bond at Price (per 100 face) on Date.
type BondCall struct {
	Date  string `json:"date"`
	Price string `json:"price"`
}

// BondTerms are the static terms analytics need. CouponRate is a decimal
// fraction (0.045 = 4.5%); prices and redemption are per 100 of face.
// CouponFrequency is coupons per year: 0 (zero coupon), 1, 2, 4 or 12.
type BondTerms struct {
	Issuer          string     `json:"issuer,omitempty"`
	IssueDate       string     `json:"issue_date,omitempty"`
	Maturity        string     `json:"maturity"`
	CouponRate      string     `json:"coupon_rate"`
	CouponFrequency *int       `json:"coupon_frequency"`
	DayCount        string     `json:"day_count"`
	FaceValue       string     `json:"face_value,omitempty"`
	Redemption      string     `json:"redemption,omitempty"`
	Calls           []BondCall `json:"calls"`
}

type NewBond struct {
	Name        string       `json:"name"`
	Currency    string       `json:"currency"`
	Identifiers []Identifier `json:"identifiers"`
	BondTerms
}

type Bond struct {
	Instrument
	Identifiers []Identifier `json:"identifiers"`
	BondTerms
}

// PriceBar is a daily bar. Bond bars are clean prices per 100 of face.
// AdjClose is empty when the source did not supply an adjusted close.
type PriceBar struct {
	InstrumentID string `json:"instrument_id"`
	SourceID     string `json:"source_id"`
	Date         string `json:"date"`
	Open         string `json:"open"`
	High         string `json:"high"`
	Low          string `json:"low"`
	Close        string `json:"close"`
	AdjClose     string `json:"adj_close,omitempty"`
	Volume       string `json:"volume"`
}

type ZeroPoint struct {
	TenorDays int32  `json:"tenor_days"`
	ZeroRate  string `json:"zero_rate"`
}

// BondAnalyticsRequest is a stateless scenario: supply exactly one of
// CleanPrice or Yield. Curve, when present, adds a Z-spread.
type BondAnalyticsRequest struct {
	Terms      BondTerms   `json:"terms"`
	Settlement string      `json:"settlement"`
	CleanPrice string      `json:"clean_price,omitempty"`
	Yield      string      `json:"yield,omitempty"`
	Curve      []ZeroPoint `json:"curve,omitempty"`
}

// StoredBondQuery prices a persisted bond. Without CleanPrice or Yield, the
// close of the latest SourceID bar on or before Settlement is used.
type StoredBondQuery struct {
	Settlement, CleanPrice, Yield, SourceID, CurveID string
}

type BondCashFlow struct {
	Date   string `json:"date"`
	Amount string `json:"amount"`
}

type PriceSource struct {
	SourceID string `json:"source_id"`
	Date     string `json:"date"`
}

type BondAnalytics struct {
	Settlement       string         `json:"settlement"`
	CleanPrice       string         `json:"clean_price"`
	DirtyPrice       string         `json:"dirty_price"`
	AccruedInterest  string         `json:"accrued_interest"`
	YieldToMaturity  string         `json:"yield_to_maturity"`
	YieldToWorst     string         `json:"yield_to_worst"`
	WorkoutDate      string         `json:"workout_date"`
	MacaulayDuration string         `json:"macaulay_duration"`
	ModifiedDuration string         `json:"modified_duration"`
	Convexity        string         `json:"convexity"`
	DV01             string         `json:"dv01"`
	ZSpread          string         `json:"z_spread,omitempty"`
	CurveID          string         `json:"curve_id,omitempty"`
	CurveObservedAt  *time.Time     `json:"curve_observed_at,omitempty"`
	PreviousCoupon   string         `json:"previous_coupon,omitempty"`
	NextCoupon       string         `json:"next_coupon,omitempty"`
	PriceSource      *PriceSource   `json:"price_source,omitempty"`
	CashFlows        []BondCashFlow `json:"cash_flows"`
}

type PerformanceQuery struct {
	SourceID, From, To, BenchmarkID, RiskFreeRate string
}

// Performance statistics use simple daily returns, annualized with 252
// trading days. PriceField is adj_close when every bar in both series has one.
type Performance struct {
	InstrumentID          string `json:"instrument_id"`
	SourceID              string `json:"source_id"`
	PriceField            string `json:"price_field"`
	Observations          int    `json:"observations"`
	StartDate             string `json:"start_date"`
	EndDate               string `json:"end_date"`
	StartPrice            string `json:"start_price"`
	EndPrice              string `json:"end_price"`
	TotalReturn           string `json:"total_return"`
	AnnualizedReturn      string `json:"annualized_return"`
	AnnualizedVolatility  string `json:"annualized_volatility"`
	MaxDrawdown           string `json:"max_drawdown"`
	RiskFreeRate          string `json:"risk_free_rate"`
	Sharpe                string `json:"sharpe,omitempty"`
	BenchmarkID           string `json:"benchmark_id,omitempty"`
	BenchmarkObservations int    `json:"benchmark_observations,omitempty"`
	Beta                  string `json:"beta,omitempty"`
	Correlation           string `json:"correlation,omitempty"`
	SMA20                 string `json:"sma_20,omitempty"`
	SMA50                 string `json:"sma_50,omitempty"`
	SMA200                string `json:"sma_200,omitempty"`
}

type IngestResult struct {
	Received int   `json:"received"`
	Inserted int64 `json:"inserted"`
}

// SecurityStore persists the security master and price history.
type SecurityStore interface {
	CreateEquity(context.Context, NewEquity) (string, error)
	CreateBond(context.Context, NewBond) (string, error)
	Bond(context.Context, string) (Bond, error)
	Bars(ctx context.Context, id, source, from, to string) ([]PriceBar, error)
	BarOnOrBefore(ctx context.Context, id, source, date string) (PriceBar, error)
	IngestBars(context.Context, []PriceBar) (int64, error)
	Points(context.Context, string) ([]CurvePoint, error)
}
