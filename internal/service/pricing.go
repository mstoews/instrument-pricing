package service

import (
	"fmt"
	"math"
	"pricing-platform/internal/domain"
	"regexp"
	"strconv"
)

type Pricer struct{ engine domain.PricingEngine }

func NewPricer(engine domain.PricingEngine) *Pricer { return &Pricer{engine: engine} }

var modelDecimal = regexp.MustCompile(`^-?[0-9]{1,12}(\.[0-9]{1,8})?$`)

func (p *Pricer) European(q domain.EuropeanPriceRequest) (domain.EuropeanPriceResult, error) {
	invalid := func(message string) (domain.EuropeanPriceResult, error) {
		return domain.EuropeanPriceResult{}, fmt.Errorf("%w: %s", domain.ErrInvalid, message)
	}
	if q.ExerciseStyle != "european" {
		return invalid("exercise_style must be european")
	}
	if q.RightType != "call" && q.RightType != "put" {
		return invalid("right_type must be call or put")
	}
	in := domain.PricingInputs{IsPut: q.RightType == "put"}
	for _, field := range []struct {
		name, value      string
		target           *float64
		positive, signed bool
	}{
		{"strike", q.Strike, &in.Strike, true, false},
		{"time_to_expiry", q.TimeToExpiry, &in.TimeToExpiry, false, false},
		{"spot", q.Spot, &in.Spot, true, false},
		{"volatility", q.Volatility, &in.Volatility, false, false},
		{"risk_free_rate", q.RiskFreeRate, &in.RiskFreeRate, false, true},
		{"dividend_yield", q.DividendYield, &in.DividendYield, false, true},
	} {
		if !modelDecimal.MatchString(field.value) {
			return invalid(field.name + " must be a decimal string with up to 12 integral and 8 fractional digits")
		}
		value, err := strconv.ParseFloat(field.value, 64)
		if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || (field.positive && value <= 0) || (!field.signed && value < 0) {
			return invalid("invalid " + field.name)
		}
		*field.target = value
	}
	price, err := p.engine.Price(in)
	if err != nil {
		return domain.EuropeanPriceResult{}, err
	}
	if math.IsNaN(price) || math.IsInf(price, 0) || price < 0 {
		return domain.EuropeanPriceResult{}, fmt.Errorf("pricing engine returned an invalid price")
	}
	return domain.EuropeanPriceResult{Model: "black_scholes", Price: strconv.FormatFloat(price, 'f', -1, 64)}, nil
}
