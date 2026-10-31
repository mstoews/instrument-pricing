package service

import (
	"context"
	"errors"
	"pricing-platform/internal/domain"
	"testing"
	"time"
)

type fakeStore struct {
	domain.Store
	calls int
}

func (f *fakeStore) IngestQuote(_ context.Context, _ domain.Quote) error { f.calls++; return nil }
func TestQuoteValidation(t *testing.T) {
	f := &fakeStore{}
	s := New(f)
	q := domain.Quote{InstrumentID: "00000000-0000-0000-0000-000000000001", SourceID: "sample", ObservedAt: time.Now(), Bid: "1.12345678", Ask: "2"}
	if e := s.IngestQuote(context.Background(), q); e != nil {
		t.Fatal(e)
	}
	for _, value := range []string{"NaN", "-1", "3", "1.123456789", "1e2"} {
		q.Bid = value
		if e := s.IngestQuote(context.Background(), q); !errors.Is(e, domain.ErrInvalid) {
			t.Errorf("accepted %s", value)
		}
	}
	if f.calls != 1 {
		t.Fatal("invalid input reached persistence")
	}
}
func (f *fakeStore) IngestCurve(context.Context, domain.CurveSnapshot) (domain.CurveIngestResult, error) {
	f.calls++
	return domain.CurveIngestResult{CurveID: "c", Inserted: true}, nil
}
func TestCurveValidation(t *testing.T) {
	f := &fakeStore{}
	s := New(f)
	good := domain.CurveSnapshot{CurveName: "UST zero", Currency: "USD", SourceID: "treasury", ObservedAt: time.Now(),
		Points: []domain.ZeroPoint{{TenorDays: 30, ZeroRate: "0.04123456"}, {TenorDays: 365, ZeroRate: "-0.001"}}}
	if _, err := s.IngestCurve(context.Background(), good); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*domain.CurveSnapshot){
		"no name":      func(x *domain.CurveSnapshot) { x.CurveName = " " },
		"bad currency": func(x *domain.CurveSnapshot) { x.Currency = "usd" },
		"no source":    func(x *domain.CurveSnapshot) { x.SourceID = "" },
		"no time":      func(x *domain.CurveSnapshot) { x.ObservedAt = time.Time{} },
		"no points":    func(x *domain.CurveSnapshot) { x.Points = nil },
		"dup tenor": func(x *domain.CurveSnapshot) {
			x.Points = append(x.Points, domain.ZeroPoint{TenorDays: 30, ZeroRate: "0.04"})
		},
		"bad rate": func(x *domain.CurveSnapshot) { x.Points = []domain.ZeroPoint{{TenorDays: 30, ZeroRate: "4.1%"}} },
	} {
		x := good
		mutate(&x)
		if _, err := s.IngestCurve(context.Background(), x); !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("%s: got %v", name, err)
		}
	}
	if f.calls != 1 {
		t.Fatalf("invalid curves reached persistence: %d", f.calls)
	}
}
func TestInvalidRequests(t *testing.T) {
	s := New(&fakeStore{})
	if _, e := s.Chain(context.Background(), "bad", "2027-01-15"); e == nil {
		t.Fatal("invalid UUID accepted")
	}
	if _, e := s.Chain(context.Background(), "00000000-0000-0000-0000-000000000001", "2027-02-30"); e == nil {
		t.Fatal("invalid date accepted")
	}
}
