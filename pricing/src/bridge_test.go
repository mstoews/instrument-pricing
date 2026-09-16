package pricing

import (
	"errors"
	"math"
	"pricing-platform/internal/domain"
	"testing"
)

func TestNativeValidation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*domain.PricingInputs)
	}{
		{"negative strike", func(in *domain.PricingInputs) { in.Strike = -1 }},
		{"NaN", func(in *domain.PricingInputs) { in.Volatility = math.NaN() }},
		{"infinite spot", func(in *domain.PricingInputs) { in.Spot = math.Inf(1) }},
		{"discount overflow", func(in *domain.PricingInputs) { in.RiskFreeRate = -1000 }},
		{"price overflow", func(in *domain.PricingInputs) { in.Volatility = math.MaxFloat64; in.TimeToExpiry = 4 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := domain.PricingInputs{Strike: 100, TimeToExpiry: 1, Spot: 100, Volatility: 0.2, RiskFreeRate: 0.05}
			tc.change(&in)
			if _, err := (Engine{}).Price(in); !errors.Is(err, domain.ErrInvalid) {
				t.Fatalf("expected invalid input, got %v", err)
			}
		})
	}
}
