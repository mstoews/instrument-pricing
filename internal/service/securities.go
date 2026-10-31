package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/big"
	"pricing-platform/internal/analytics"
	"pricing-platform/internal/domain"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Securities owns the security master, price history and analytics use cases.
type Securities struct{ store domain.SecurityStore }

func NewSecurities(s domain.SecurityStore) *Securities { return &Securities{s} }

const maxBarsPerRequest = 5000

var (
	scheme = regexp.MustCompile(`^[a-z][a-z0-9_]{0,29}$`)
	isin   = regexp.MustCompile(`^[A-Z]{2}[A-Z0-9]{9}[0-9]$`)
	cusip  = regexp.MustCompile(`^[0-9A-Z*@#]{8}[0-9]$`)
	volume = regexp.MustCompile(`^[0-9]{1,20}(\.[0-9]{1,4})?$`)
)

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{domain.ErrInvalid}, args...)...)
}

// alnum maps 0-9 to 0-9 and A-Z to 10-35, as used by ISIN and CUSIP check digits.
func alnum(r rune) int {
	if r >= '0' && r <= '9' {
		return int(r - '0')
	}
	return int(r-'A') + 10
}

func validISIN(s string) bool {
	if !isin.MatchString(s) {
		return false
	}
	var digits strings.Builder
	for _, r := range s {
		digits.WriteString(strconv.Itoa(alnum(r)))
	}
	sum, d := 0, digits.String()
	for i := len(d) - 1; i >= 0; i-- {
		v := int(d[i] - '0')
		if (len(d)-1-i)%2 == 1 {
			if v *= 2; v > 9 {
				v -= 9
			}
		}
		sum += v
	}
	return sum%10 == 0
}

func validCUSIP(s string) bool {
	if !cusip.MatchString(s) {
		return false
	}
	sum := 0
	for i, r := range s[:8] {
		v := map[rune]int{'*': 36, '@': 37, '#': 38}[r]
		if v == 0 {
			v = alnum(r)
		}
		if i%2 == 1 {
			v *= 2
		}
		sum += v/10 + v%10
	}
	return (10-sum%10)%10 == int(s[8]-'0')
}

func validateIdentifiers(ids []domain.Identifier) error {
	if len(ids) == 0 || len(ids) > 20 {
		return invalid("between 1 and 20 identifiers are required")
	}
	for _, x := range ids {
		switch {
		case !scheme.MatchString(x.Scheme):
			return invalid("identifier scheme must be lowercase, e.g. ticker, isin, cusip")
		case x.Value == "" || len(x.Value) > 100 || strings.TrimSpace(x.Value) != x.Value || len(x.Venue) > 30:
			return invalid("identifier value must be 1-100 characters without surrounding spaces")
		case x.Scheme == "isin" && !validISIN(x.Value):
			return invalid("invalid ISIN %q", x.Value)
		case x.Scheme == "cusip" && !validCUSIP(x.Value):
			return invalid("invalid CUSIP %q", x.Value)
		}
	}
	return nil
}

func validateName(name, ccy string) error {
	if strings.TrimSpace(name) == "" || len(name) > 200 {
		return invalid("name must be 1-200 characters")
	}
	if !currency.MatchString(ccy) {
		return invalid("currency must be a three-letter code")
	}
	return nil
}

func (s *Securities) CreateEquity(c context.Context, e domain.NewEquity) (string, error) {
	if err := validateName(e.Name, e.Currency); err != nil {
		return "", err
	}
	if e.Exchange == "" || len(e.Exchange) > 30 || len(e.Country) != 2 || len(e.Sector) > 100 {
		return "", invalid("exchange (1-30 chars) and two-letter country are required")
	}
	if err := validateIdentifiers(e.Identifiers); err != nil {
		return "", err
	}
	return s.store.CreateEquity(c, e)
}

// parseDecimal converts a validated boundary decimal string into a float.
func parseDecimal(name, v string, signed bool) (float64, error) {
	re := decimal
	if signed {
		re = modelDecimal
	}
	if !re.MatchString(v) {
		return 0, invalid("%s must be a decimal string with up to 12 integral and 8 fractional digits", name)
	}
	f, _ := strconv.ParseFloat(v, 64)
	return f, nil
}

func parseDate(name, v string) (time.Time, error) {
	t, err := analytics.ParseDate(v)
	if err != nil {
		return t, invalid("%s must be an ISO 8601 date", name)
	}
	return t, nil
}

func positive(name, v string) (float64, error) {
	f, err := parseDecimal(name, v, false)
	if err == nil && f <= 0 {
		err = invalid("%s must be positive", name)
	}
	return f, err
}

// bondModel validates terms and fills defaults (redemption 100).
func bondModel(t *domain.BondTerms) (analytics.Bond, error) {
	var b analytics.Bond
	var err error
	if t.Redemption == "" {
		t.Redemption = "100"
	}
	if t.CouponFrequency == nil {
		return b, invalid("coupon_frequency is required")
	}
	b.Frequency, b.DayCount = *t.CouponFrequency, t.DayCount
	if b.Maturity, err = parseDate("maturity", t.Maturity); err != nil {
		return b, err
	}
	if t.IssueDate != "" {
		if b.Issue, err = parseDate("issue_date", t.IssueDate); err != nil {
			return b, err
		}
		if !b.Issue.Before(b.Maturity) {
			return b, invalid("issue_date must precede maturity")
		}
	}
	if b.Coupon, err = parseDecimal("coupon_rate", t.CouponRate, false); err != nil {
		return b, err
	}
	if b.Redemption, err = positive("redemption", t.Redemption); err != nil {
		return b, err
	}
	if !analytics.ValidDayCount(b.DayCount) {
		return b, invalid("day_count must be 30/360, ACT/360, ACT/365F or ACT/ACT")
	}
	if f := b.Frequency; f != 0 && f != 1 && f != 2 && f != 4 && f != 12 {
		return b, invalid("coupon_frequency must be 0, 1, 2, 4 or 12")
	}
	if b.Frequency == 0 && b.Coupon != 0 {
		return b, invalid("zero-coupon bonds (coupon_frequency 0) must have coupon_rate 0")
	}
	if len(t.Calls) > 200 {
		return b, invalid("at most 200 calls")
	}
	seen := map[string]bool{}
	for _, c := range t.Calls {
		d, err := parseDate("call date", c.Date)
		if err != nil {
			return b, err
		}
		p, err := positive("call price", c.Price)
		if err != nil {
			return b, err
		}
		if !d.Before(b.Maturity) || seen[c.Date] {
			return b, invalid("call dates must be unique and before maturity")
		}
		seen[c.Date] = true
		b.Calls = append(b.Calls, analytics.Call{Date: d, Price: p})
	}
	if t.Calls == nil {
		t.Calls = []domain.BondCall{}
	}
	return b, nil
}

func (s *Securities) CreateBond(c context.Context, b domain.NewBond) (string, error) {
	if err := validateName(b.Name, b.Currency); err != nil {
		return "", err
	}
	if strings.TrimSpace(b.Issuer) == "" || len(b.Issuer) > 200 {
		return "", invalid("issuer must be 1-200 characters")
	}
	if _, err := positive("face_value", b.FaceValue); err != nil {
		return "", err
	}
	if _, err := bondModel(&b.BondTerms); err != nil {
		return "", err
	}
	if err := validateIdentifiers(b.Identifiers); err != nil {
		return "", err
	}
	return s.store.CreateBond(c, b)
}

func (s *Securities) Bond(c context.Context, id string) (domain.Bond, error) {
	if !uuid.MatchString(id) {
		return domain.Bond{}, domain.ErrInvalid
	}
	return s.store.Bond(c, id)
}

func validateRange(from, to string) error {
	f, err := parseDate("from", from)
	if err != nil {
		return err
	}
	t, err := parseDate("to", to)
	if err != nil {
		return err
	}
	if t.Before(f) || t.Sub(f) > 50*366*24*time.Hour {
		return invalid("from must not be after to, and the range is limited to 50 years")
	}
	return nil
}

func validSource(id string) bool { return id != "" && len(id) <= 50 }

func (s *Securities) Bars(c context.Context, id, source, from, to string) ([]domain.PriceBar, error) {
	if !uuid.MatchString(id) || !validSource(source) {
		return nil, invalid("valid instrument id and source are required")
	}
	if err := validateRange(from, to); err != nil {
		return nil, err
	}
	return s.store.Bars(c, id, source, from, to)
}

func cmpDecimal(a, b string) int {
	x, _ := new(big.Rat).SetString(a)
	y, _ := new(big.Rat).SetString(b)
	return x.Cmp(y)
}

func (s *Securities) IngestBars(c context.Context, bars []domain.PriceBar) (domain.IngestResult, error) {
	if len(bars) == 0 || len(bars) > maxBarsPerRequest {
		return domain.IngestResult{}, invalid("between 1 and %d bars are required", maxBarsPerRequest)
	}
	for i, b := range bars {
		if !uuid.MatchString(b.InstrumentID) || !validSource(b.SourceID) {
			return domain.IngestResult{}, invalid("bar %d: valid instrument_id and source_id are required", i)
		}
		if _, err := parseDate("date", b.Date); err != nil {
			return domain.IngestResult{}, invalid("bar %d: date must be an ISO 8601 date", i)
		}
		for _, v := range []string{b.Open, b.High, b.Low, b.Close} {
			if !decimal.MatchString(v) {
				return domain.IngestResult{}, invalid("bar %d: prices must be non-negative decimal strings", i)
			}
		}
		if b.AdjClose != "" {
			if _, err := positive("adj_close", b.AdjClose); err != nil {
				return domain.IngestResult{}, invalid("bar %d: adj_close must be a positive decimal string", i)
			}
		}
		if b.Volume == "" {
			bars[i].Volume = "0"
		} else if !volume.MatchString(b.Volume) {
			return domain.IngestResult{}, invalid("bar %d: volume must be a non-negative decimal string", i)
		}
		if cmpDecimal(b.Low, b.Open) > 0 || cmpDecimal(b.Low, b.Close) > 0 || cmpDecimal(b.Open, b.High) > 0 || cmpDecimal(b.Close, b.High) > 0 {
			return domain.IngestResult{}, invalid("bar %d: low <= open, close <= high is required", i)
		}
	}
	n, err := s.store.IngestBars(c, bars)
	return domain.IngestResult{Received: len(bars), Inserted: n}, err
}

func format(v float64) (string, error) {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return "", errors.New("analytics produced a non-finite value")
	}
	return strconv.FormatFloat(v, 'f', -1, 64), nil
}

// formatAll formats float fields into their string targets, failing on non-finite values.
func formatAll(pairs ...any) error {
	for i := 0; i < len(pairs); i += 2 {
		v, err := format(pairs[i+1].(float64))
		if err != nil {
			return err
		}
		*pairs[i].(*string) = v
	}
	return nil
}

func analyticsError(err error) error {
	if errors.Is(err, analytics.ErrInput) {
		return fmt.Errorf("%w: %s", domain.ErrInvalid, strings.TrimPrefix(err.Error(), analytics.ErrInput.Error()+": "))
	}
	return err
}

func zeroCurve(points []domain.ZeroPoint) ([]analytics.CurvePoint, error) {
	if len(points) > 200 {
		return nil, invalid("at most 200 curve points")
	}
	out := make([]analytics.CurvePoint, 0, len(points))
	seen := map[int32]bool{}
	for _, p := range points {
		r, err := parseDecimal("zero_rate", p.ZeroRate, true)
		if err != nil {
			return nil, err
		}
		if p.TenorDays <= 0 || seen[p.TenorDays] {
			return nil, invalid("curve tenor_days must be positive and unique")
		}
		seen[p.TenorDays] = true
		out = append(out, analytics.CurvePoint{TenorDays: int(p.TenorDays), ZeroRate: r})
	}
	return out, nil
}

func (s *Securities) analyze(terms domain.BondTerms, settlement, clean, yield string, curve []analytics.CurvePoint) (domain.BondAnalytics, error) {
	b, err := bondModel(&terms)
	if err != nil {
		return domain.BondAnalytics{}, err
	}
	settle, err := parseDate("settlement", settlement)
	if err != nil {
		return domain.BondAnalytics{}, err
	}
	var cp, yp *float64
	if (clean == "") == (yield == "") {
		return domain.BondAnalytics{}, invalid("supply exactly one of clean_price or yield")
	}
	if clean != "" {
		v, err := positive("clean_price", clean)
		if err != nil {
			return domain.BondAnalytics{}, err
		}
		cp = &v
	} else {
		v, err := parseDecimal("yield", yield, true)
		if err != nil {
			return domain.BondAnalytics{}, err
		}
		yp = &v
	}
	r, err := analytics.AnalyzeBond(b, settle, cp, yp, curve)
	if err != nil {
		return domain.BondAnalytics{}, analyticsError(err)
	}
	out := domain.BondAnalytics{Settlement: settlement, WorkoutDate: analytics.FormatDate(r.WorkoutDate), CashFlows: []domain.BondCashFlow{}}
	if err := formatAll(&out.CleanPrice, r.CleanPrice, &out.DirtyPrice, r.DirtyPrice, &out.AccruedInterest, r.Accrued,
		&out.YieldToMaturity, r.YieldToMaturity, &out.YieldToWorst, r.YieldToWorst, &out.MacaulayDuration, r.MacaulayDuration,
		&out.ModifiedDuration, r.ModifiedDuration, &out.Convexity, r.Convexity, &out.DV01, r.DV01); err != nil {
		return domain.BondAnalytics{}, err
	}
	if r.ZSpread != nil {
		if out.ZSpread, err = format(*r.ZSpread); err != nil {
			return domain.BondAnalytics{}, err
		}
	}
	if !r.PreviousCoupon.IsZero() {
		out.PreviousCoupon, out.NextCoupon = analytics.FormatDate(r.PreviousCoupon), analytics.FormatDate(r.NextCoupon)
	}
	for _, cf := range r.CashFlows {
		amount, err := format(cf.Amount)
		if err != nil {
			return domain.BondAnalytics{}, err
		}
		out.CashFlows = append(out.CashFlows, domain.BondCashFlow{Date: analytics.FormatDate(cf.Date), Amount: amount})
	}
	return out, nil
}

// BondAnalytics prices an explicit scenario without persisted data.
func (s *Securities) BondAnalytics(q domain.BondAnalyticsRequest) (domain.BondAnalytics, error) {
	curve, err := zeroCurve(q.Curve)
	if err != nil {
		return domain.BondAnalytics{}, err
	}
	return s.analyze(q.Terms, q.Settlement, q.CleanPrice, q.Yield, curve)
}

// StoredBondAnalytics prices a persisted bond, optionally from a stored price and curve.
func (s *Securities) StoredBondAnalytics(c context.Context, id string, q domain.StoredBondQuery) (domain.BondAnalytics, error) {
	if !uuid.MatchString(id) || (q.CurveID != "" && !uuid.MatchString(q.CurveID)) {
		return domain.BondAnalytics{}, invalid("malformed id")
	}
	if _, err := parseDate("settlement", q.Settlement); err != nil {
		return domain.BondAnalytics{}, err
	}
	bond, err := s.store.Bond(c, id)
	if err != nil {
		return domain.BondAnalytics{}, err
	}
	var src *domain.PriceSource
	if q.CleanPrice == "" && q.Yield == "" {
		if !validSource(q.SourceID) {
			return domain.BondAnalytics{}, invalid("supply clean_price, yield, or source to use a stored price")
		}
		bar, err := s.store.BarOnOrBefore(c, id, q.SourceID, q.Settlement)
		if errors.Is(err, domain.ErrNotFound) {
			return domain.BondAnalytics{}, fmt.Errorf("%w: no %s price on or before %s", domain.ErrNotFound, q.SourceID, q.Settlement)
		}
		if err != nil {
			return domain.BondAnalytics{}, err
		}
		q.CleanPrice, src = bar.Close, &domain.PriceSource{SourceID: bar.SourceID, Date: bar.Date}
	}
	var curve []analytics.CurvePoint
	var observed *time.Time
	if q.CurveID != "" {
		points, err := s.store.Points(c, q.CurveID)
		if err != nil {
			return domain.BondAnalytics{}, err
		}
		if len(points) == 0 {
			return domain.BondAnalytics{}, fmt.Errorf("%w: curve has no snapshot", domain.ErrNotFound)
		}
		for _, p := range points {
			r, err := strconv.ParseFloat(p.ZeroRate, 64)
			if err != nil {
				return domain.BondAnalytics{}, err
			}
			curve = append(curve, analytics.CurvePoint{TenorDays: int(p.TenorDays), ZeroRate: r})
		}
		observed = &points[0].ObservedAt
	}
	out, err := s.analyze(bond.BondTerms, q.Settlement, q.CleanPrice, q.Yield, curve)
	if err != nil {
		return out, err
	}
	out.PriceSource, out.CurveObservedAt = src, observed
	if q.CurveID != "" {
		out.CurveID = q.CurveID
	}
	return out, nil
}

// closes returns the series using adj_close only when every bar supplies it.
func closes(bars []domain.PriceBar, useAdj bool) []analytics.PricePoint {
	out := make([]analytics.PricePoint, 0, len(bars))
	for _, b := range bars {
		v := b.Close
		if useAdj {
			v = b.AdjClose
		}
		d, _ := analytics.ParseDate(b.Date)
		f, _ := strconv.ParseFloat(v, 64)
		out = append(out, analytics.PricePoint{Date: d, Close: f})
	}
	return out
}

func allAdjusted(series ...[]domain.PriceBar) bool {
	for _, bars := range series {
		for _, b := range bars {
			if b.AdjClose == "" {
				return false
			}
		}
	}
	return true
}

func (s *Securities) Performance(c context.Context, id string, q domain.PerformanceQuery) (domain.Performance, error) {
	if q.BenchmarkID != "" && !uuid.MatchString(q.BenchmarkID) {
		return domain.Performance{}, invalid("malformed benchmark_id")
	}
	if q.RiskFreeRate == "" {
		q.RiskFreeRate = "0"
	}
	rf, err := parseDecimal("risk_free_rate", q.RiskFreeRate, true)
	if err != nil {
		return domain.Performance{}, err
	}
	bars, err := s.Bars(c, id, q.SourceID, q.From, q.To)
	if err != nil {
		return domain.Performance{}, err
	}
	var bench []domain.PriceBar
	if q.BenchmarkID != "" {
		if bench, err = s.store.Bars(c, q.BenchmarkID, q.SourceID, q.From, q.To); err != nil {
			return domain.Performance{}, err
		}
	}
	adj := allAdjusted(bars, bench)
	var benchSeries []analytics.PricePoint
	if q.BenchmarkID != "" {
		if benchSeries = closes(bench, adj); len(benchSeries) == 0 {
			return domain.Performance{}, invalid("benchmark has no %s prices in range", q.SourceID)
		}
	}
	r, err := analytics.AnalyzeEquity(closes(bars, adj), benchSeries, rf)
	if err != nil {
		return domain.Performance{}, analyticsError(err)
	}
	out := domain.Performance{InstrumentID: id, SourceID: q.SourceID, PriceField: "close", Observations: r.Observations,
		StartDate: analytics.FormatDate(r.Start.Date), EndDate: analytics.FormatDate(r.End.Date), RiskFreeRate: q.RiskFreeRate,
		BenchmarkID: q.BenchmarkID, BenchmarkObservations: r.BenchmarkObservations}
	if adj {
		out.PriceField = "adj_close"
	}
	if err := formatAll(&out.StartPrice, r.Start.Close, &out.EndPrice, r.End.Close, &out.TotalReturn, r.TotalReturn,
		&out.AnnualizedReturn, r.AnnualizedReturn, &out.AnnualizedVolatility, r.AnnualizedVolatility, &out.MaxDrawdown, r.MaxDrawdown); err != nil {
		return domain.Performance{}, err
	}
	for _, opt := range []struct {
		target *string
		value  *float64
	}{{&out.Sharpe, r.Sharpe}, {&out.Beta, r.Beta}, {&out.Correlation, r.Correlation}, {&out.SMA20, r.SMA20}, {&out.SMA50, r.SMA50}, {&out.SMA200, r.SMA200}} {
		if opt.value != nil {
			if *opt.target, err = format(*opt.value); err != nil {
				return domain.Performance{}, err
			}
		}
	}
	return out, nil
}
