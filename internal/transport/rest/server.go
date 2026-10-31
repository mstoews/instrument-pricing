package rest

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"pricing-platform/internal/domain"
	"pricing-platform/internal/service"
)

func reply(w http.ResponseWriter, v any, e error) { replyStatus(w, http.StatusOK, v, e) }

func replyStatus(w http.ResponseWriter, ok int, v any, e error) {
	w.Header().Set("Content-Type", "application/json")
	if e != nil {
		status := 500
		message := "internal server error"
		if errors.Is(e, domain.ErrInvalid) {
			status = 400
			message = e.Error()
		} else if errors.Is(e, domain.ErrNotFound) {
			status = 404
			message = e.Error()
		} else if errors.Is(e, domain.ErrUnauthorized) {
			status = 401
			message = e.Error()
			w.Header().Set("WWW-Authenticate", `Bearer realm="pricing"`)
		} else if errors.Is(e, domain.ErrForbidden) {
			status = 403
			message = e.Error()
		} else if errors.Is(e, domain.ErrRateLimited) {
			status = 429
			message = e.Error()
			w.Header().Set("Retry-After", "900")
		} else if errors.Is(e, domain.ErrConflict) {
			status = 409
			message = "conflict: an identifier or record already exists"
		} else {
			slog.Error("request failed", "error", e)
		}
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
		return
	}
	w.WriteHeader(ok)
	_ = json.NewEncoder(w).Encode(v)
}

// decode reads exactly one JSON document with no unknown fields.
func decode(w http.ResponseWriter, r *http.Request, limit int64, v any) bool {
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, limit))
	d.DisallowUnknownFields()
	var extra any
	if d.Decode(v) != nil || d.Decode(&extra) != io.EOF {
		reply(w, nil, domain.ErrInvalid)
		return false
	}
	return true
}

// Handler routes the REST API. securities may be nil to omit security
// master, history and analytics routes. authn nil disables authentication
// (tests and explicit local development only); otherwise every route
// declares the permission it requires.
func Handler(s *service.Market, pricer *service.Pricer, securities *service.Securities, authn *service.Auth) http.Handler {
	m := http.NewServeMux()
	rt := router{m, authn}
	if authn != nil {
		routeAuth(rt, authn)
	}
	if securities != nil {
		routeSecurities(rt, securities)
	}
	rt.handle("POST /v1/pricing/european", domain.PermRead, func(w http.ResponseWriter, r *http.Request) {
		var q domain.EuropeanPriceRequest
		if !decode(w, r, 16384, &q) {
			return
		}
		v, err := pricer.European(q)
		reply(w, v, err)
	})
	rt.handle("GET /healthz", domain.PermPublic, func(w http.ResponseWriter, r *http.Request) { reply(w, map[string]string{"status": "ok"}, nil) })
	rt.handle("GET /v1/instruments", domain.PermRead, func(w http.ResponseWriter, r *http.Request) {
		v, e := s.Search(r.Context(), r.URL.Query().Get("q"))
		reply(w, v, e)
	})
	rt.handle("GET /v1/instruments/resolve", domain.PermRead, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		v, e := s.Resolve(r.Context(), q.Get("scheme"), q.Get("value"), q.Get("venue"))
		reply(w, v, e)
	})
	rt.handle("GET /v1/market-data/{id}/quote", domain.PermRead, func(w http.ResponseWriter, r *http.Request) {
		v, e := s.Quote(r.Context(), r.PathValue("id"))
		reply(w, v, e)
	})
	rt.handle("GET /v1/options/{id}/expirations", domain.PermRead, func(w http.ResponseWriter, r *http.Request) {
		v, e := s.Expirations(r.Context(), r.PathValue("id"))
		reply(w, v, e)
	})
	rt.handle("GET /v1/options/{id}/chain", domain.PermRead, func(w http.ResponseWriter, r *http.Request) {
		v, e := s.Chain(r.Context(), r.PathValue("id"), r.URL.Query().Get("expiration"))
		reply(w, v, e)
	})
	rt.handle("GET /v1/curves", domain.PermRead, func(w http.ResponseWriter, r *http.Request) {
		v, e := s.Curves(r.Context(), r.URL.Query().Get("currency"))
		reply(w, v, e)
	})
	rt.handle("GET /v1/curves/{id}/points", domain.PermRead, func(w http.ResponseWriter, r *http.Request) {
		v, e := s.Points(r.Context(), r.PathValue("id"))
		reply(w, v, e)
	})
	rt.handle("POST /v1/ingestion/quotes", domain.PermWrite, func(w http.ResponseWriter, r *http.Request) {
		var q domain.Quote
		if !decode(w, r, 16384, &q) {
			return
		}
		e := s.IngestQuote(r.Context(), q)
		reply(w, map[string]string{"status": "accepted"}, e)
	})
	for _, path := range []string{"trades", "curves"} {
		rt.handle("POST /v1/ingestion/"+path, domain.PermWrite, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotImplemented)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "ingestion adapter not implemented in v0.1"})
		})
	}
	return m
}
