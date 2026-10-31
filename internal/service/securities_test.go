package service

import (
	"context"
	"errors"
	"pricing-platform/internal/domain"
	"strconv"
	"testing"
)

type fakeSecurities struct {
	domain.SecurityStore
	bars   map[string][]domain.PriceBar
	bond   domain.Bond
	writes int
}

func (f *fakeSecurities) CreateBond(context.Context, domain.NewBond) (string, error) {
	f.writes++
	return "id", nil
}
func (f *fakeSecurities) CreateEquity(context.Context, domain.NewEquity) (string, error) {
	f.writes++
	return "id", nil
}
func (f *fakeSecurities) Bond(context.Context, string) (domain.Bond, error) { return f.bond, nil }
func (f *fakeSecurities) Bars(_ context.Context, id, _, _, _ string) ([]domain.PriceBar, error) {
	return f.bars[id], nil
}
func (f *fakeSecurities) BarOnOrBefore(context.Context, string, string, string) (domain.PriceBar, error) {
	return domain.PriceBar{SourceID: "sample", Date: "2026-01-14", Close: "80.20722612"}, nil
}
func (f *fakeSecurities) IngestBars(_ context.Context, b []domain.PriceBar) (int64, error) {
	f.writes++
	return int64(len(b)), nil
}

const (
	a = "00000000-0000-0000-0000-000000000001"
	b = "00000000-0000-0000-0000-000000000002"
)

func freq(n int) *int { return &n }

func terms() domain.BondTerms {
	return domain.BondTerms{Maturity: "2046-01-15", CouponRate: "0.06", CouponFrequency: freq(2), DayCount: "30/360"}
}

func TestIdentifierCheckDigits(t *testing.T) {
	for _, ok := range []string{"US0378331005", "US912828Z781", "GB0002634946"} {
		if !validISIN(ok) {
			t.Errorf("rejected ISIN %s", ok)
		}
	}
	for _, bad := range []string{"US0378331006", "us0378331005", "US037833100"} {
		if validISIN(bad) {
			t.Errorf("accepted ISIN %s", bad)
		}
	}
	if !validCUSIP("037833100") || !validCUSIP("912828Z78") || validCUSIP("037833101") {
		t.Error("CUSIP check digit")
	}
}

func TestCreateBondValidation(t *testing.T) {
	f := &fakeSecurities{}
	s := NewSecurities(f)
	good := domain.NewBond{Name: "Sample 6% 2046", Currency: "USD", Identifiers: []domain.Identifier{{Scheme: "cusip", Value: "037833100"}}, BondTerms: terms()}
	good.Issuer, good.FaceValue = "Sample Issuer", "1000"
	if _, err := s.CreateBond(context.Background(), good); err != nil {
		t.Fatal(err)
	}
	mutations := map[string]func(*domain.NewBond){
		"no identifiers": func(x *domain.NewBond) { x.Identifiers = nil },
		"bad cusip":      func(x *domain.NewBond) { x.Identifiers = []domain.Identifier{{Scheme: "cusip", Value: "037833101"}} },
		"no frequency":   func(x *domain.NewBond) { x.CouponFrequency = nil },
		"bad frequency":  func(x *domain.NewBond) { x.CouponFrequency = freq(3) },
		"zero w/ coupon": func(x *domain.NewBond) { x.CouponFrequency = freq(0) },
		"bad day count":  func(x *domain.NewBond) { x.DayCount = "BUS/252" },
		"percent coupon": func(x *domain.NewBond) { x.CouponRate = "6%" },
		"no face":        func(x *domain.NewBond) { x.FaceValue = "" },
		"call at expiry": func(x *domain.NewBond) { x.Calls = []domain.BondCall{{Date: "2046-01-15", Price: "100"}} },
		"dup calls": func(x *domain.NewBond) {
			x.Calls = []domain.BondCall{{Date: "2030-01-15", Price: "100"}, {Date: "2030-01-15", Price: "101"}}
		},
		"issue after mat": func(x *domain.NewBond) { x.IssueDate = "2047-01-01" },
		"bad currency":    func(x *domain.NewBond) { x.Currency = "usd" },
	}
	for name, mutate := range mutations {
		x := good
		mutate(&x)
		if _, err := s.CreateBond(context.Background(), x); !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("%s: got %v", name, err)
		}
	}
	if f.writes != 1 {
		t.Fatalf("invalid bonds reached persistence: %d writes", f.writes)
	}
}

func TestBondAnalyticsScenario(t *testing.T) {
	s := NewSecurities(&fakeSecurities{})
	r, err := s.BondAnalytics(domain.BondAnalyticsRequest{Terms: terms(), Settlement: "2026-01-15", Yield: "0.08",
		Curve: []domain.ZeroPoint{{TenorDays: 365, ZeroRate: "0.04"}, {TenorDays: 7300, ZeroRate: "0.05"}}})
	if err != nil {
		t.Fatal(err)
	}
	price, _ := strconv.ParseFloat(r.CleanPrice, 64)
	if price < 80.207226 || price > 80.207227 || r.WorkoutDate != "2046-01-15" || r.ZSpread == "" || len(r.CashFlows) != 40 {
		t.Fatalf("unexpected result %+v", r)
	}
	for name, q := range map[string]domain.BondAnalyticsRequest{
		"both":      {Terms: terms(), Settlement: "2026-01-15", Yield: "0.08", CleanPrice: "99"},
		"neither":   {Terms: terms(), Settlement: "2026-01-15"},
		"matured":   {Terms: terms(), Settlement: "2046-01-15", CleanPrice: "99"},
		"bad curve": {Terms: terms(), Settlement: "2026-01-15", CleanPrice: "99", Curve: []domain.ZeroPoint{{TenorDays: 0, ZeroRate: "0.04"}}},
		"bad date":  {Terms: terms(), Settlement: "2026-02-30", CleanPrice: "99"},
		"exp yield": {Terms: terms(), Settlement: "2026-01-15", Yield: "8e-2"},
	} {
		if _, err := s.BondAnalytics(q); !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("%s: got %v", name, err)
		}
	}
}

func TestStoredBondUsesLatestBar(t *testing.T) {
	s := NewSecurities(&fakeSecurities{bond: domain.Bond{BondTerms: terms()}})
	r, err := s.StoredBondAnalytics(context.Background(), a, domain.StoredBondQuery{Settlement: "2026-01-15", SourceID: "sample"})
	if err != nil {
		t.Fatal(err)
	}
	if r.PriceSource == nil || r.PriceSource.Date != "2026-01-14" || r.CleanPrice != "80.20722612" {
		t.Fatalf("price source %+v / %s", r.PriceSource, r.CleanPrice)
	}
	if _, err := s.StoredBondAnalytics(context.Background(), a, domain.StoredBondQuery{Settlement: "2026-01-15"}); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal("missing price input accepted")
	}
}

func bar(id, date, close, adj string) domain.PriceBar {
	return domain.PriceBar{InstrumentID: id, SourceID: "sample", Date: date, Open: close, High: close, Low: close, Close: close, AdjClose: adj, Volume: "10"}
}

func TestPerformance(t *testing.T) {
	f := &fakeSecurities{bars: map[string][]domain.PriceBar{
		a: {bar(a, "2026-01-02", "100", "50"), bar(a, "2026-01-05", "110", "55"), bar(a, "2026-01-06", "99", "49.5")},
		b: {bar(b, "2026-01-02", "10", ""), bar(b, "2026-01-05", "10.5", ""), bar(b, "2026-01-06", "10", "")},
	}}
	s := NewSecurities(f)
	r, err := s.Performance(context.Background(), a, domain.PerformanceQuery{SourceID: "sample", From: "2026-01-01", To: "2026-01-31"})
	if err != nil {
		t.Fatal(err)
	}
	if r.PriceField != "adj_close" || r.TotalReturn != "-0.010000000000000009" || r.Beta != "" {
		t.Fatalf("unexpected %+v", r)
	}
	// The benchmark lacks adj_close, so both series fall back to close.
	r, err = s.Performance(context.Background(), a, domain.PerformanceQuery{SourceID: "sample", From: "2026-01-01", To: "2026-01-31", BenchmarkID: b, RiskFreeRate: "0.04"})
	if err != nil || r.PriceField != "close" || r.Beta == "" || r.BenchmarkObservations != 3 {
		t.Fatalf("benchmark %+v %v", r, err)
	}
	for name, q := range map[string]domain.PerformanceQuery{
		"no source":     {From: "2026-01-01", To: "2026-01-31"},
		"reversed":      {SourceID: "sample", From: "2026-02-01", To: "2026-01-31"},
		"bad benchmark": {SourceID: "sample", From: "2026-01-01", To: "2026-01-31", BenchmarkID: "x"},
		"no history":    {SourceID: "sample", From: "2026-01-01", To: "2026-01-31", BenchmarkID: "00000000-0000-0000-0000-000000000009"},
	} {
		if _, err := s.Performance(context.Background(), a, q); !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("%s: got %v", name, err)
		}
	}
}

func TestIngestBarsValidation(t *testing.T) {
	f := &fakeSecurities{}
	s := NewSecurities(f)
	good := bar(a, "2026-01-02", "100", "")
	good.High, good.Low, good.Volume = "101", "99", ""
	r, err := s.IngestBars(context.Background(), []domain.PriceBar{good})
	if err != nil || r.Inserted != 1 {
		t.Fatalf("%+v %v", r, err)
	}
	for name, mutate := range map[string]func(*domain.PriceBar){
		"high below close": func(x *domain.PriceBar) { x.High = "99.5" },
		"negative":         func(x *domain.PriceBar) { x.Low = "-1" },
		"bad date":         func(x *domain.PriceBar) { x.Date = "2026-13-01" },
		"bad volume":       func(x *domain.PriceBar) { x.Volume = "1e6" },
		"zero adj":         func(x *domain.PriceBar) { x.AdjClose = "0" },
		"no source":        func(x *domain.PriceBar) { x.SourceID = "" },
	} {
		x := good
		mutate(&x)
		if _, err := s.IngestBars(context.Background(), []domain.PriceBar{x}); !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("%s: got %v", name, err)
		}
	}
	if _, err := s.IngestBars(context.Background(), nil); !errors.Is(err, domain.ErrInvalid) {
		t.Error("empty batch accepted")
	}
	if f.writes != 1 {
		t.Fatalf("invalid bars reached persistence: %d", f.writes)
	}
}
