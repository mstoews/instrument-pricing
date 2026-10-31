package analytics

import (
	"math"
	"time"
)

// TradingDays annualizes daily return statistics.
const TradingDays = 252

type PricePoint struct {
	Date  time.Time
	Close float64
}

type EquityResult struct {
	Observations                      int
	Start, End                        PricePoint
	TotalReturn, AnnualizedReturn     float64
	AnnualizedVolatility, MaxDrawdown float64
	Sharpe                            *float64 // nil when volatility is zero
	Beta, Correlation                 *float64 // nil without a benchmark
	BenchmarkObservations             int
	SMA20, SMA50, SMA200              *float64 // nil without enough history
}

func returns(s []PricePoint) []float64 {
	out := make([]float64, 0, len(s)-1)
	for i := 1; i < len(s); i++ {
		out = append(out, s[i].Close/s[i-1].Close-1)
	}
	return out
}

func meanStd(x []float64) (mean, std float64) {
	for _, v := range x {
		mean += v
	}
	mean /= float64(len(x))
	for _, v := range x {
		std += (v - mean) * (v - mean)
	}
	return mean, math.Sqrt(std / float64(len(x)-1))
}

func sma(s []PricePoint, n int) *float64 {
	if len(s) < n {
		return nil
	}
	sum := 0.0
	for _, p := range s[len(s)-n:] {
		sum += p.Close
	}
	v := sum / float64(n)
	return &v
}

// align keeps only dates present in both date-ascending series.
func align(a, b []PricePoint) ([]PricePoint, []PricePoint) {
	var x, y []PricePoint
	for i, j := 0, 0; i < len(a) && j < len(b); {
		switch {
		case a[i].Date.Equal(b[j].Date):
			x, y = append(x, a[i]), append(y, b[j])
			i, j = i+1, j+1
		case a[i].Date.Before(b[j].Date):
			i++
		default:
			j++
		}
	}
	return x, y
}

// AnalyzeEquity computes return and risk statistics from date-ascending,
// strictly positive closes using simple daily returns. riskFree is an annual
// decimal rate; benchmark is optional and aligned on common dates.
func AnalyzeEquity(series, benchmark []PricePoint, riskFree float64) (EquityResult, error) {
	if len(series) < 3 {
		return EquityResult{}, errInput("at least 3 prices are required")
	}
	for i, p := range series {
		if p.Close <= 0 || (i > 0 && !p.Date.After(series[i-1].Date)) {
			return EquityResult{}, errInput("prices must be positive with strictly increasing dates")
		}
	}
	start, end := series[0], series[len(series)-1]
	r := EquityResult{Observations: len(series), Start: start, End: end, TotalReturn: end.Close/start.Close - 1}
	if days := actualDays(start.Date, end.Date); days > 0 {
		r.AnnualizedReturn = math.Pow(end.Close/start.Close, 365/days) - 1
	}
	daily := returns(series)
	mean, std := meanStd(daily)
	r.AnnualizedVolatility = std * math.Sqrt(TradingDays)
	if r.AnnualizedVolatility > 0 {
		s := (mean*TradingDays - riskFree) / r.AnnualizedVolatility
		r.Sharpe = &s
	}
	peak := start.Close
	for _, p := range series {
		peak = math.Max(peak, p.Close)
		r.MaxDrawdown = math.Min(r.MaxDrawdown, p.Close/peak-1)
	}
	r.SMA20, r.SMA50, r.SMA200 = sma(series, 20), sma(series, 50), sma(series, 200)
	if len(benchmark) > 0 {
		x, y := align(series, benchmark)
		r.BenchmarkObservations = len(x)
		if len(x) < 3 {
			return EquityResult{}, errInput("benchmark shares fewer than 3 dates with the series")
		}
		for _, p := range y {
			if p.Close <= 0 {
				return EquityResult{}, errInput("benchmark prices must be positive")
			}
		}
		rx, ry := returns(x), returns(y)
		mx, sx := meanStd(rx)
		my, sy := meanStd(ry)
		cov := 0.0
		for i := range rx {
			cov += (rx[i] - mx) * (ry[i] - my)
		}
		cov /= float64(len(rx) - 1)
		if sy > 0 {
			beta := cov / (sy * sy)
			r.Beta = &beta
			if sx > 0 {
				corr := cov / (sx * sy)
				r.Correlation = &corr
			}
		}
	}
	return r, nil
}
