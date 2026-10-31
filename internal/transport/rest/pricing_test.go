package rest

import (
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"pricing-platform/internal/domain"
	"pricing-platform/internal/service"
	pricing "pricing-platform/pricing/src"
	"strconv"
	"strings"
	"testing"
)

func pricingPayload() map[string]any {
	return map[string]any{"exercise_style": "european", "right_type": "call", "strike": "100", "time_to_expiry": "1", "spot": "100", "volatility": "0.20", "risk_free_rate": "0.05", "dividend_yield": "0"}
}

func priceHTTP(t *testing.T, h http.Handler, payload map[string]any) float64 {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "/v1/pricing/european", strings.NewReader(string(body))))
	if w.Code != 200 {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	if w.Header().Get("Content-Type") != "application/json" {
		t.Fatal("missing JSON content type")
	}
	var result domain.EuropeanPriceResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Model != "black_scholes" {
		t.Fatalf("unexpected model %q", result.Model)
	}
	price, err := strconv.ParseFloat(result.Price, 64)
	if err != nil || math.IsNaN(price) || math.IsInf(price, 0) {
		t.Fatalf("invalid price %q", result.Price)
	}
	return price
}

// These tests cross REST validation, the service, cgo, and the actual C++ engine.
func TestEuropeanPricingHTTP(t *testing.T) {
	h := Handler(service.New(fakeStore{}), service.NewPricer(pricing.Engine{}), nil)
	for _, tc := range []struct {
		name    string
		changes map[string]any
		want    float64
	}{
		{"call", nil, 10.450583572},
		{"put", map[string]any{"right_type": "put"}, 5.573526022},
		{"expired call", map[string]any{"time_to_expiry": "0", "strike": "90"}, 10},
		{"expired put", map[string]any{"time_to_expiry": "0", "strike": "110", "right_type": "put"}, 10},
		{"zero volatility", map[string]any{"volatility": "0", "dividend_yield": "0.02"}, 100*math.Exp(-0.02) - 100*math.Exp(-0.05)},
		{"negative rate", map[string]any{"volatility": "0", "risk_free_rate": "-0.05", "right_type": "put"}, 100*math.Exp(0.05) - 100},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			payload := pricingPayload()
			for k, v := range tc.changes {
				payload[k] = v
			}
			if got := priceHTTP(t, h, payload); math.Abs(got-tc.want) > 1e-8 {
				t.Fatalf("price %.12f, want %.12f", got, tc.want)
			}
		})
	}
}

func TestEuropeanDividendParityHTTP(t *testing.T) {
	h := Handler(service.New(fakeStore{}), service.NewPricer(pricing.Engine{}), nil)
	payload := pricingPayload()
	payload["dividend_yield"] = "0.02"
	call := priceHTTP(t, h, payload)
	payload["right_type"] = "put"
	put := priceHTTP(t, h, payload)
	want := 100*math.Exp(-0.02) - 100*math.Exp(-0.05)
	if math.Abs(call-put-want) > 1e-10 {
		t.Fatalf("put-call parity: %g, want %g", call-put, want)
	}
}

func TestEuropeanPricingRejectsInvalidHTTP(t *testing.T) {
	h := Handler(service.New(fakeStore{}), service.NewPricer(pricing.Engine{}), nil)
	reject := func(t *testing.T, body string) {
		t.Helper()
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("POST", "/v1/pricing/european", strings.NewReader(body)))
		if w.Code != 400 {
			t.Fatalf("status %d: %s", w.Code, w.Body.String())
		}
		var result map[string]string
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || result["error"] == "" {
			t.Fatalf("invalid error response: %s", w.Body.String())
		}
	}
	for _, field := range []string{"exercise_style", "right_type", "strike", "time_to_expiry", "spot", "volatility", "risk_free_rate", "dividend_yield"} {
		for _, kind := range []string{"missing", "null", "number", "empty"} {
			t.Run(field+"/"+kind, func(t *testing.T) {
				payload := pricingPayload()
				switch kind {
				case "missing":
					delete(payload, field)
				case "null":
					payload[field] = nil
				case "number":
					payload[field] = 1
				case "empty":
					payload[field] = ""
				}
				body, _ := json.Marshal(payload)
				reject(t, string(body))
			})
		}
	}
	for _, tc := range []struct{ field, value string }{
		{"exercise_style", "american"}, {"right_type", "CALL"},
		{"strike", "0"}, {"spot", "0"}, {"strike", "-1"}, {"spot", "-1"},
		{"volatility", "-0.1"}, {"time_to_expiry", "-1"},
		{"spot", "NaN"}, {"risk_free_rate", "Inf"}, {"strike", "1e2"},
		{"spot", "1000000000000"}, {"spot", "0.123456789"}, {"spot", " 100"},
		{"risk_free_rate", "-1000"}, // Native discount overflow must not escape the ABI.
		{"vendor_payload", "unexpected"},
	} {
		t.Run(tc.field+"/"+tc.value, func(t *testing.T) {
			payload := pricingPayload()
			payload[tc.field] = tc.value
			body, _ := json.Marshal(payload)
			reject(t, string(body))
		})
	}
	body, _ := json.Marshal(pricingPayload())
	for _, raw := range []string{"", "null", "[]", "{}", "{", string(body) + " {}", string(body) + " null", strings.Repeat(" ", 16385) + string(body)} {
		reject(t, raw)
	}
	// A native rejection leaves the engine usable for the next request.
	priceHTTP(t, h, pricingPayload())
}

type brokenEngine struct{}

func (brokenEngine) Price(domain.PricingInputs) (float64, error) {
	return 0, errors.New("private engine failure")
}

func TestEuropeanPricingInternalError(t *testing.T) {
	h := Handler(service.New(fakeStore{}), service.NewPricer(brokenEngine{}), nil)
	body, _ := json.Marshal(pricingPayload())
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "/v1/pricing/european", strings.NewReader(string(body))))
	if w.Code != 500 || strings.TrimSpace(w.Body.String()) != `{"error":"internal server error"}` {
		t.Fatalf("unexpected response: %d %s", w.Code, w.Body.String())
	}
}
