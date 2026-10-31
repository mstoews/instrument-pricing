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

type fakeStore struct{ domain.Store }

func (f fakeStore) Quote(context.Context, string) (domain.Quote, error) {
	return domain.Quote{}, domain.ErrNotFound
}
func TestHTTPFailures(t *testing.T) {
	h := Handler(service.New(fakeStore{}), service.NewPricer(pricing.Engine{}), nil, nil)
	for _, tc := range []struct {
		method, path, body string
		status             int
	}{
		{"GET", "/v1/market-data/bad/quote", "", 400},
		{"GET", "/v1/market-data/00000000-0000-0000-0000-000000000999/quote", "", 404},
		{"POST", "/v1/ingestion/quotes", `{"vendor_payload":{}}`, 400},
		{"POST", "/v1/ingestion/quotes", `{} {}`, 400},
		{"POST", "/v1/ingestion/trades", `{}`, 501},
		{"POST", "/v1/ingestion/curves", `{"curve_name":"UST","currency":"USD","source_id":"treasury","observed_at":"2026-10-30T19:30:00Z","points":[]}`, 400},
		{"POST", "/v1/ingestion/curves", `{"curve_name":"UST","currency":"USD","source_id":"treasury","observed_at":"2026-10-30T19:30:00Z","points":[{"tenor_days":30,"zero_rate":"4.1%"}]}`, 400},
	} {
		r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Errorf("%s: got %d want %d", tc.path, w.Code, tc.status)
		}
	}
}
