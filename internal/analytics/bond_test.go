package analytics

import (
	"errors"
	"math"
	"testing"
	"time"
)

func d(s string) time.Time   { t, _ := ParseDate(s); return t }
func ptr(v float64) *float64 { return &v }
func near(t *testing.T, name string, got, want, tol float64) {
	t.Helper()
	if math.Abs(got-want) > tol {
		t.Errorf("%s = %.12f, want %.12f", name, got, want)
	}
}

func TestAnalyzeBondReferenceValues(t *testing.T) {
	// 20-year 6% semiannual 30/360 at an 8% yield, settling on a coupon date.
	b := Bond{Maturity: d("2046-01-15"), Coupon: 0.06, Frequency: 2, DayCount: Thirty360, Redemption: 100}
	r, err := AnalyzeBond(b, d("2026-01-15"), nil, ptr(0.08), nil)
	if err != nil {
		t.Fatal(err)
	}
	near(t, "accrued", r.Accrued, 0, 1e-12)
	near(t, "price", r.CleanPrice, 80.20722611657348, 1e-9)
	near(t, "macaulay", r.MacaulayDuration, 10.922490979950224, 1e-9)
	near(t, "modified", r.ModifiedDuration, 10.502395173029061, 1e-9)
	near(t, "convexity", r.Convexity, 162.31949287616345, 1e-7)
	near(t, "dv01", r.DV01, 10.502395173029061*80.20722611657348/10000, 1e-12)
	if len(r.CashFlows) != 40 || r.CashFlows[39].Amount != 103 {
		t.Fatalf("unexpected cash flows: %d", len(r.CashFlows))
	}
	back, err := AnalyzeBond(b, d("2026-01-15"), ptr(r.CleanPrice), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	near(t, "round-trip yield", back.YieldToMaturity, 0.08, 1e-12)
}

func TestAccruedAndParYield(t *testing.T) {
	b := Bond{Maturity: d("2031-07-15"), Coupon: 0.06, Frequency: 2, DayCount: Thirty360, Redemption: 100}
	r, err := AnalyzeBond(b, d("2026-03-01"), ptr(100), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if FormatDate(r.PreviousCoupon) != "2026-01-15" || FormatDate(r.NextCoupon) != "2026-07-15" {
		t.Fatalf("period %s..%s", FormatDate(r.PreviousCoupon), FormatDate(r.NextCoupon))
	}
	near(t, "accrued", r.Accrued, 3*46.0/180, 1e-12) // 30/360: Jan 15 → Mar 1 is 46 days
	near(t, "dirty", r.DirtyPrice, 100+3*46.0/180, 1e-12)
	// Trading at par clean between coupons yields slightly under the coupon on a street basis.
	if r.YieldToMaturity >= 0.06 || r.YieldToMaturity < 0.0599 {
		t.Fatalf("ytm %.10f", r.YieldToMaturity)
	}
	par, _ := AnalyzeBond(b, d("2026-01-15"), ptr(100), nil, nil)
	near(t, "par ytm on coupon date", par.YieldToMaturity, 0.06, 1e-12)
}

func TestZeroCoupon(t *testing.T) {
	b := Bond{Maturity: d("2028-06-30"), Frequency: 0, DayCount: Thirty360, Redemption: 100}
	r, err := AnalyzeBond(b, d("2026-06-30"), nil, ptr(0.05), nil)
	if err != nil {
		t.Fatal(err)
	}
	near(t, "price", r.CleanPrice, 100/math.Pow(1.025, 4), 1e-10)
	near(t, "macaulay", r.MacaulayDuration, 2, 1e-12)
	near(t, "modified", r.ModifiedDuration, 2/1.025, 1e-12)
}

func TestEndOfMonthSchedule(t *testing.T) {
	b := Bond{Maturity: d("2030-08-31"), Coupon: 0.04, Frequency: 2, DayCount: ActAct, Redemption: 100}
	prev, next := b.schedule(d("2028-03-10"))
	if FormatDate(prev) != "2028-02-29" || FormatDate(next[0]) != "2028-08-31" || FormatDate(next[1]) != "2029-02-28" {
		t.Fatalf("schedule %s %s %s", FormatDate(prev), FormatDate(next[0]), FormatDate(next[1]))
	}
}

func TestYieldToWorstOnPremiumCallable(t *testing.T) {
	b := Bond{Maturity: d("2036-01-15"), Coupon: 0.06, Frequency: 2, DayCount: Thirty360, Redemption: 100,
		Calls: []Call{{d("2025-01-15"), 101}, {d("2028-01-15"), 100}}}
	r, err := AnalyzeBond(b, d("2026-01-15"), ptr(105), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if FormatDate(r.WorkoutDate) != "2028-01-15" || r.YieldToWorst >= r.YieldToMaturity {
		t.Fatalf("ytw %.6f at %s, ytm %.6f", r.YieldToWorst, FormatDate(r.WorkoutDate), r.YieldToMaturity)
	}
	// Called at 100 on a coupon date: 4 coupons of 3 plus 100, priced at 105.
	want := dirtyFromYield([]CashFlow{{Amount: 3, Periods: 1}, {Amount: 3, Periods: 2}, {Amount: 3, Periods: 3}, {Amount: 103, Periods: 4}}, r.YieldToWorst, 2)
	near(t, "ytc reprices", want, 105, 1e-9)
	discount, _ := AnalyzeBond(b, d("2026-01-15"), ptr(95), nil, nil)
	if !discount.WorkoutDate.Equal(b.Maturity) {
		t.Fatal("discount bond should be worked out to maturity")
	}
}

func TestZSpreadRecoversFlatSpread(t *testing.T) {
	b := Bond{Maturity: d("2031-05-15"), Coupon: 0.045, Frequency: 2, DayCount: ActAct, Redemption: 100}
	settle := d("2026-08-03")
	curve := []CurvePoint{{3650, 0.04}, {30, 0.04}}
	base, err := AnalyzeBond(b, settle, ptr(100), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	dirty := 0.0
	for _, cf := range base.CashFlows {
		tt := actualDays(settle, cf.Date) / 365
		dirty += cf.Amount * math.Exp(-(0.04+0.0125)*tt)
	}
	r, err := AnalyzeBond(b, settle, ptr(dirty-base.Accrued), nil, curve)
	if err != nil {
		t.Fatal(err)
	}
	near(t, "z-spread", *r.ZSpread, 0.0125, 1e-10)
}

func TestBondValidation(t *testing.T) {
	b := Bond{Maturity: d("2030-01-01"), Coupon: 0.05, Frequency: 2, DayCount: Thirty360, Redemption: 100}
	for name, tc := range map[string]struct {
		b      Bond
		settle string
		clean  *float64
		y      *float64
	}{
		"matured":      {b, "2030-01-01", ptr(100), nil},
		"both inputs":  {b, "2026-01-01", ptr(100), ptr(0.05)},
		"no inputs":    {b, "2026-01-01", nil, nil},
		"bad freq":     {Bond{Maturity: b.Maturity, Coupon: 0.05, Frequency: 3, DayCount: Thirty360, Redemption: 100}, "2026-01-01", ptr(100), nil},
		"bad dc":       {Bond{Maturity: b.Maturity, Coupon: 0.05, Frequency: 2, DayCount: "BUS/252", Redemption: 100}, "2026-01-01", ptr(100), nil},
		"zero coupon":  {Bond{Maturity: b.Maturity, Coupon: 0.05, Frequency: 0, DayCount: Thirty360, Redemption: 100}, "2026-01-01", ptr(100), nil},
		"pre-issue":    {Bond{Issue: d("2027-01-01"), Maturity: b.Maturity, Coupon: 0.05, Frequency: 2, DayCount: Thirty360, Redemption: 100}, "2026-01-01", ptr(100), nil},
		"absurd price": {b, "2026-01-01", ptr(0.0001), nil},
	} {
		if _, err := AnalyzeBond(tc.b, d(tc.settle), tc.clean, tc.y, nil); !errors.Is(err, ErrInput) {
			t.Errorf("%s: got %v", name, err)
		}
	}
}
