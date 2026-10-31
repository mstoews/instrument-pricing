package analytics

import (
	"errors"
	"testing"
)

func series(closes ...float64) []PricePoint {
	out := make([]PricePoint, len(closes))
	start := d("2026-01-01")
	for i, c := range closes {
		out[i] = PricePoint{start.AddDate(0, 0, i), c}
	}
	return out
}

func TestAnalyzeEquity(t *testing.T) {
	r, err := AnalyzeEquity(series(100, 110, 99, 120), nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	near(t, "total", r.TotalReturn, 0.2, 1e-12)
	near(t, "max drawdown", r.MaxDrawdown, 99.0/110-1, 1e-12)
	if r.Sharpe == nil || r.Beta != nil || r.SMA20 != nil {
		t.Fatal("unexpected optional fields")
	}
	flat, _ := AnalyzeEquity(series(100, 101, 102.01), nil, 0.03)
	near(t, "volatility", flat.AnnualizedVolatility, 0, 1e-12)
	if flat.Sharpe != nil {
		t.Fatal("sharpe must be nil at zero volatility")
	}
}

func TestBetaAndCorrelation(t *testing.T) {
	bench := series(100, 101, 99, 102, 100)
	lever := []float64{100}
	for i := 1; i < len(bench); i++ {
		lever = append(lever, lever[i-1]*(1+2*(bench[i].Close/bench[i-1].Close-1)))
	}
	r, err := AnalyzeEquity(series(lever...), bench, 0)
	if err != nil {
		t.Fatal(err)
	}
	near(t, "beta", *r.Beta, 2, 1e-12)
	near(t, "correlation", *r.Correlation, 1, 1e-12)
	// Alignment drops dates missing from either series.
	gappy := append(append([]PricePoint{}, bench[:2]...), bench[3:]...)
	r, err = AnalyzeEquity(bench, gappy, 0)
	if err != nil || r.BenchmarkObservations != 4 {
		t.Fatalf("aligned %d, %v", r.BenchmarkObservations, err)
	}
	near(t, "self beta", *r.Beta, 1, 1e-12)
}

func TestSMAAndValidation(t *testing.T) {
	closes := make([]float64, 50)
	for i := range closes {
		closes[i] = float64(i + 1)
	}
	r, _ := AnalyzeEquity(series(closes...), nil, 0)
	near(t, "sma20", *r.SMA20, 40.5, 1e-12)
	near(t, "sma50", *r.SMA50, 25.5, 1e-12)
	if r.SMA200 != nil {
		t.Fatal("sma200 without history")
	}
	bad := series(1, 2, 3)
	bad[2].Date = bad[1].Date
	for _, s := range [][]PricePoint{series(1, 2), series(1, 0, 2), bad} {
		if _, err := AnalyzeEquity(s, nil, 0); !errors.Is(err, ErrInput) {
			t.Errorf("accepted %v", s)
		}
	}
	if _, err := AnalyzeEquity(series(1, 2, 3), series(1, 2), 0); !errors.Is(err, ErrInput) {
		t.Error("accepted short benchmark")
	}
}
