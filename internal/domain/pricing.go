package domain

// EuropeanPriceRequest is a scenario, independent of persisted market data.
// Numeric inputs are decimal strings; time is in years and rates are continuously compounded.
type EuropeanPriceRequest struct {
	ExerciseStyle string `json:"exercise_style"`
	RightType     string `json:"right_type"`
	Strike        string `json:"strike"`
	TimeToExpiry  string `json:"time_to_expiry"`
	Spot          string `json:"spot"`
	Volatility    string `json:"volatility"`
	RiskFreeRate  string `json:"risk_free_rate"`
	DividendYield string `json:"dividend_yield"`
}

// PricingInputs contains the floating-point model inputs after boundary validation.
type PricingInputs struct {
	IsPut                                                               bool
	Strike, TimeToExpiry, Spot, Volatility, RiskFreeRate, DividendYield float64
}

type PricingEngine interface {
	Price(PricingInputs) (float64, error)
}

type EuropeanPriceResult struct {
	Model string `json:"model"`
	Price string `json:"price"`
}
