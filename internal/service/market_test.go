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
func TestInvalidRequests(t *testing.T) {
	s := New(&fakeStore{})
	if _, e := s.Chain(context.Background(), "bad", "2027-01-15"); e == nil {
		t.Fatal("invalid UUID accepted")
	}
	if _, e := s.Chain(context.Background(), "00000000-0000-0000-0000-000000000001", "2027-02-30"); e == nil {
		t.Fatal("invalid date accepted")
	}
}
