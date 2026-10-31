package rest

import (
	"context"
	"net/http/httptest"
	"pricing-platform/internal/domain"
	"pricing-platform/internal/service"
	pricing "pricing-platform/pricing/src"
	"strings"
	"testing"
)

type fakeSecurities struct{ domain.SecurityStore }

func (fakeSecurities) CreateEquity(context.Context, domain.NewEquity) (string, error) {
	return "", domain.ErrConflict
}
func (fakeSecurities) CreateBond(context.Context, domain.NewBond) (string, error) {
	return "00000000-0000-0000-0000-000000000042", nil
}

func TestSecuritiesHTTP(t *testing.T) {
	h := Handler(service.New(fakeStore{}), service.NewPricer(pricing.Engine{}), service.NewSecurities(fakeSecurities{}))
	bond := `{"name":"Sample 5% 2031","currency":"USD","issuer":"Sample","maturity":"2031-01-15","coupon_rate":"0.05","coupon_frequency":2,"day_count":"30/360","face_value":"1000","calls":[],"identifiers":[{"scheme":"isin","value":"US0378331005","venue":""}]}`
	scenario := `{"terms":{"maturity":"2031-01-15","coupon_rate":"0.05","coupon_frequency":2,"day_count":"30/360"},"settlement":"2026-01-15","clean_price":"100"}`
	for _, tc := range []struct {
		method, path, body string
		status             int
		contains           string
	}{
		{"POST", "/v1/instruments/bonds", bond, 201, `"id":"00000000-0000-0000-0000-000000000042"`},
		{"POST", "/v1/instruments/equities", `{"name":"X","currency":"USD","exchange":"XNAS","country":"US","sector":"","identifiers":[{"scheme":"ticker","value":"X","venue":"XNAS"}]}`, 409, "conflict"},
		{"POST", "/v1/instruments/equities", `{"name":"X","ticker":"X"}`, 400, "invalid"},
		{"POST", "/v1/analytics/bond", scenario, 200, `"yield_to_maturity":"0.05`},
		{"POST", "/v1/analytics/bond", strings.Replace(scenario, `"clean_price":"100"`, `"yield":"0.05","clean_price":"100"`, 1), 400, "exactly one"},
		{"GET", "/v1/market-data/00000000-0000-0000-0000-000000000001/bars?from=2026-01-01&to=2026-02-01", "", 400, "source"},
		{"GET", "/v1/bonds/not-a-uuid", "", 400, "invalid"},
		{"POST", "/v1/ingestion/bars", `{"bars":[]}`, 400, "bars are required"},
	} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body)))
		if w.Code != tc.status || !strings.Contains(w.Body.String(), tc.contains) {
			t.Errorf("%s %s: got %d %s", tc.method, tc.path, w.Code, w.Body.String())
		}
	}
}
