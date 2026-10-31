package rest

import (
	"context"
	"net/http/httptest"
	"pricing-platform/internal/auth"
	"pricing-platform/internal/domain"
	"pricing-platform/internal/service"
	pricing "pricing-platform/pricing/src"
	"strings"
	"testing"
	"time"
)

// fakeAuthStore resolves any well-formed access token to a regular user and
// any API key to an ingest-scoped key.
type fakeAuthStore struct{ domain.AuthStore }

func (fakeAuthStore) SessionByAccess(context.Context, []byte) (domain.Principal, error) {
	return domain.Principal{UserID: "u", Role: domain.RoleUser, FamilyID: "f"}, nil
}
func (fakeAuthStore) APIKeyByHash(context.Context, []byte) (domain.Principal, *time.Time, error) {
	now := time.Now()
	return domain.Principal{UserID: "u", Scope: domain.ScopeIngest, APIKeyID: "k"}, &now, nil
}

func TestRoutePermissions(t *testing.T) {
	a := service.NewAuth(fakeAuthStore{}, service.AuthConfig{})
	h := Handler(service.New(fakeStore{}), service.NewPricer(pricing.Engine{}), service.NewSecurities(fakeSecurities{}), a)
	user, _, _ := auth.NewToken(auth.AccessPrefix)
	key, _, _ := auth.NewToken(auth.APIKeyPrefix)
	quote := "/v1/market-data/00000000-0000-0000-0000-000000000999/quote"
	for _, tc := range []struct {
		method, path, token, body string
		status                    int
	}{
		{"GET", "/healthz", "", "", 200},
		{"GET", quote, "", "", 401},
		{"GET", quote, "ipa_garbage", "", 401},
		{"GET", quote, user, "", 404}, // authenticated; the fake market store has no quote
		{"POST", "/v1/ingestion/quotes", user, `{}`, 403},
		{"POST", "/v1/instruments/bonds", user, `{}`, 403},
		{"POST", "/v1/ingestion/quotes", key, `{}`, 400}, // allowed through to validation
		{"POST", "/v1/auth/api-keys", key, `{"name":"x","scope":"read"}`, 403},
		{"GET", "/v1/auth/api-keys", user, "", 403},
		{"POST", "/v1/auth/register", "", `{"email":"a@b.co","password":"long enough password"}`, 403}, // sign-up disabled
		{"POST", "/v1/auth/login", "", `{"email":"a@b.co"} {}`, 400},
		{"POST", "/v1/auth/refresh", "", `{"refresh_token":"nope"}`, 401},
	} {
		r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		if tc.token != "" {
			r.Header.Set("Authorization", "Bearer "+tc.token)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Errorf("%s %s: got %d want %d: %s", tc.method, tc.path, w.Code, tc.status, w.Body.String())
		}
		if w.Code == 401 && w.Header().Get("WWW-Authenticate") == "" {
			t.Errorf("%s: 401 without WWW-Authenticate", tc.path)
		}
	}
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/v1/auth/me", nil)
	r.Header.Set("Authorization", "Bearer "+key)
	h.ServeHTTP(w, r)
	if !strings.Contains(w.Body.String(), `"credential":"api_key"`) || !strings.Contains(w.Body.String(), `"scope":"ingest"`) {
		t.Fatalf("me: %s", w.Body.String())
	}
}
