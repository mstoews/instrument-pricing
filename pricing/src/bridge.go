// Package pricing adapts the standalone C++ engine to Go. It lives beside the
// C++ sources so the Go build compiles and tracks the canonical implementation.
package pricing

/*
#cgo CXXFLAGS: -std=c++17 -I${SRCDIR}/../include
#cgo linux LDFLAGS: -lstdc++ -lm
#cgo darwin LDFLAGS: -lc++
#cgo windows LDFLAGS: -lstdc++
#include "bridge.h"
*/
import "C"

import (
	"fmt"
	"pricing-platform/internal/domain"
)

type Engine struct{}

var _ domain.PricingEngine = Engine{}

func (Engine) Price(in domain.PricingInputs) (float64, error) {
	var isPut C.int
	if in.IsPut {
		isPut = 1
	}
	var price C.double
	status := C.pricing_european(isPut, C.double(in.Strike), C.double(in.TimeToExpiry),
		C.double(in.Spot), C.double(in.Volatility), C.double(in.RiskFreeRate), C.double(in.DividendYield), &price)
	switch status {
	case 0:
		return float64(price), nil
	case 1:
		return 0, fmt.Errorf("%w: invalid pricing inputs", domain.ErrInvalid)
	case 2:
		return 0, fmt.Errorf("%w: pricing inputs exceed the model's numerical range", domain.ErrInvalid)
	default:
		return 0, fmt.Errorf("C++ pricing engine failed (status %d)", int(status))
	}
}
