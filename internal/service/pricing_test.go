package service

import (
	"errors"
	"math"
	"pricing-platform/internal/domain"
	"testing"
)

type pricingEngineFunc func(domain.PricingInputs) (float64, error)

func (f pricingEngineFunc) Price(in domain.PricingInputs) (float64, error) { return f(in) }

func validScenario() domain.EuropeanPriceRequest {
	return domain.EuropeanPriceRequest{ExerciseStyle: "european", RightType: "put", Strike: "100.12345678", TimeToExpiry: "0", Spot: "101", Volatility: "0", RiskFreeRate: "-0.05", DividendYield: "-0.02"}
}

func TestPricingValidationBeforeEngine(t *testing.T) {
	calls := 0
	p := NewPricer(pricingEngineFunc(func(in domain.PricingInputs) (float64, error) {
		calls++
		want := domain.PricingInputs{IsPut: true, Strike: 100.12345678, Spot: 101, RiskFreeRate: -0.05, DividendYield: -0.02}
		if in != want {
			t.Fatalf("got %+v, want %+v", in, want)
		}
		return 0, nil
	}))
	q := validScenario()
	q.ExerciseStyle = "american"
	if _, err := p.European(q); !errors.Is(err, domain.ErrInvalid) || calls != 0 {
		t.Fatalf("invalid scenario reached engine: calls=%d err=%v", calls, err)
	}
	result, err := p.European(validScenario())
	if err != nil || calls != 1 || result.Price != "0" {
		t.Fatalf("result=%+v calls=%d err=%v", result, calls, err)
	}
}

func TestPricingRejectsInvalidEngineResult(t *testing.T) {
	for _, price := range []float64{math.NaN(), math.Inf(1), math.Inf(-1), -1} {
		p := NewPricer(pricingEngineFunc(func(domain.PricingInputs) (float64, error) { return price, nil }))
		if _, err := p.European(validScenario()); err == nil || errors.Is(err, domain.ErrInvalid) {
			t.Fatalf("engine result %g should produce an internal error, got %v", price, err)
		}
	}
}
