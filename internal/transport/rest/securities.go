package rest

import (
	"net/http"
	"pricing-platform/internal/domain"
	"pricing-platform/internal/service"
)

func routeSecurities(rt router, s *service.Securities) {
	rt.handle("POST /v1/instruments/equities", domain.PermWrite, func(w http.ResponseWriter, r *http.Request) {
		var q domain.NewEquity
		if !decode(w, r, 65536, &q) {
			return
		}
		id, e := s.CreateEquity(r.Context(), q)
		replyStatus(w, http.StatusCreated, map[string]string{"id": id}, e)
	})
	rt.handle("POST /v1/instruments/bonds", domain.PermWrite, func(w http.ResponseWriter, r *http.Request) {
		var q domain.NewBond
		if !decode(w, r, 65536, &q) {
			return
		}
		id, e := s.CreateBond(r.Context(), q)
		replyStatus(w, http.StatusCreated, map[string]string{"id": id}, e)
	})
	rt.handle("GET /v1/bonds/{id}", domain.PermRead, func(w http.ResponseWriter, r *http.Request) {
		v, e := s.Bond(r.Context(), r.PathValue("id"))
		reply(w, v, e)
	})
	rt.handle("GET /v1/bonds/{id}/analytics", domain.PermRead, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		v, e := s.StoredBondAnalytics(r.Context(), r.PathValue("id"), domain.StoredBondQuery{Settlement: q.Get("settlement"),
			CleanPrice: q.Get("clean_price"), Yield: q.Get("yield"), SourceID: q.Get("source"), CurveID: q.Get("curve_id")})
		reply(w, v, e)
	})
	rt.handle("POST /v1/analytics/bond", domain.PermRead, func(w http.ResponseWriter, r *http.Request) {
		var q domain.BondAnalyticsRequest
		if !decode(w, r, 65536, &q) {
			return
		}
		v, e := s.BondAnalytics(q)
		reply(w, v, e)
	})
	rt.handle("GET /v1/market-data/{id}/bars", domain.PermRead, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		v, e := s.Bars(r.Context(), r.PathValue("id"), q.Get("source"), q.Get("from"), q.Get("to"))
		reply(w, v, e)
	})
	rt.handle("GET /v1/instruments/{id}/performance", domain.PermRead, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		v, e := s.Performance(r.Context(), r.PathValue("id"), domain.PerformanceQuery{SourceID: q.Get("source"), From: q.Get("from"),
			To: q.Get("to"), BenchmarkID: q.Get("benchmark_id"), RiskFreeRate: q.Get("risk_free_rate")})
		reply(w, v, e)
	})
	rt.handle("POST /v1/ingestion/bars", domain.PermWrite, func(w http.ResponseWriter, r *http.Request) {
		var q struct {
			Bars []domain.PriceBar `json:"bars"`
		}
		if !decode(w, r, 2<<20, &q) {
			return
		}
		v, e := s.IngestBars(r.Context(), q.Bars)
		reply(w, v, e)
	})
}
