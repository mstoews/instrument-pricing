package analytics

import (
	"errors"
	"math"
	"sort"
	"time"
)

// ErrInput reports analytics inputs that cannot produce a result.
var ErrInput = errors.New("invalid analytics input")

type Call struct {
	Date  time.Time
	Price float64 // per 100 face
}

// Bond is a fixed-rate bullet bond with an optional discrete call schedule.
// Coupon is an annual decimal rate; Frequency 0 means zero coupon.
type Bond struct {
	Issue      time.Time // optional; zero when unknown
	Maturity   time.Time
	Coupon     float64
	Frequency  int
	DayCount   string
	Redemption float64 // per 100 face
	Calls      []Call
}

type CashFlow struct {
	Date    time.Time
	Amount  float64 // per 100 face
	Periods float64 // discounting exponent in compounding periods
}

// CurvePoint is a continuously compounded zero rate at a tenor from settlement.
type CurvePoint struct {
	TenorDays int
	ZeroRate  float64
}

type BondResult struct {
	CleanPrice, DirtyPrice, Accrued    float64
	YieldToMaturity, YieldToWorst      float64
	WorkoutDate                        time.Time
	MacaulayDuration, ModifiedDuration float64
	Convexity, DV01                    float64
	ZSpread                            *float64
	PreviousCoupon, NextCoupon         time.Time // zero for zero-coupon bonds
	CashFlows                          []CashFlow
}

func (b Bond) compounding() int {
	if b.Frequency == 0 {
		return 2 // street convention for zero-coupon yields
	}
	return b.Frequency
}

func (b Bond) validate(settle time.Time) error {
	switch {
	case !ValidDayCount(b.DayCount):
		return errInput("unsupported day count %q", b.DayCount)
	case b.Frequency != 0 && b.Frequency != 1 && b.Frequency != 2 && b.Frequency != 4 && b.Frequency != 12:
		return errInput("coupon frequency must be 0, 1, 2, 4 or 12")
	case b.Frequency == 0 && b.Coupon != 0:
		return errInput("zero-coupon bonds must have a zero coupon rate")
	case b.Coupon < 0 || b.Redemption <= 0:
		return errInput("coupon must be non-negative and redemption positive")
	case !settle.Before(b.Maturity):
		return errInput("settlement must be before maturity")
	case !b.Issue.IsZero() && settle.Before(b.Issue):
		return errInput("settlement must not precede issue")
	}
	return nil
}

// schedule returns the regular coupon period containing settle and the
// remaining coupon dates (> settle) through maturity, generated backward.
func (b Bond) schedule(settle time.Time) (prev time.Time, next []time.Time) {
	step := 12 / b.Frequency
	eom := isMonthEnd(b.Maturity)
	d := b.Maturity
	for k := 1; d.After(settle); k++ {
		next = append(next, d)
		d = addMonths(b.Maturity, -k*step, eom)
	}
	for i, j := 0, len(next)-1; i < j; i, j = i+1, j-1 {
		next[i], next[j] = next[j], next[i]
	}
	return d, next
}

// accrued returns accrued interest per 100 at settle.
func (b Bond) accrued(settle time.Time) (float64, time.Time, time.Time) {
	if b.Frequency == 0 {
		return 0, time.Time{}, time.Time{}
	}
	prev, next := b.schedule(settle)
	start := prev
	if !b.Issue.IsZero() && b.Issue.After(start) {
		start = b.Issue
	}
	f := float64(b.Frequency)
	return b.Coupon * 100 / f * periodFraction(b.DayCount, start, settle, prev, next[0], b.Frequency), prev, next[0]
}

// flows returns cash flows to a workout date (maturity or a call date) that
// redeems at price; a call between coupon dates pays the accrued coupon.
func (b Bond) flows(settle, workout time.Time, price float64) []CashFlow {
	if b.Frequency == 0 {
		return []CashFlow{{workout, price, yearFraction(b.DayCount, settle, workout) * 2}}
	}
	prev, next := b.schedule(settle)
	coupon := b.Coupon * 100 / float64(b.Frequency)
	w := periodFraction(b.DayCount, settle, next[0], prev, next[0], b.Frequency)
	var out []CashFlow
	start := prev
	for j, c := range next {
		if !c.Before(workout) {
			var t float64
			if j == 0 {
				t = periodFraction(b.DayCount, settle, workout, prev, c, b.Frequency)
			} else {
				t = w + float64(j-1) + periodFraction(b.DayCount, start, workout, start, c, b.Frequency)
			}
			// A workout on a coupon date pays the full coupon; between coupon
			// dates it pays the coupon accrued since the period start (or issue).
			amt := price + coupon
			if workout.Before(c) {
				from := start
				if j == 0 && !b.Issue.IsZero() && b.Issue.After(start) {
					from = b.Issue
				}
				amt = price + coupon*periodFraction(b.DayCount, from, workout, start, c, b.Frequency)
			}
			return append(out, CashFlow{workout, amt, t})
		}
		out = append(out, CashFlow{c, coupon, w + float64(j)})
		start = c
	}
	return out
}

func dirtyFromYield(flows []CashFlow, y float64, f int) float64 {
	v := 1 / (1 + y/float64(f))
	p := 0.0
	for _, cf := range flows {
		p += cf.Amount * math.Pow(v, cf.Periods)
	}
	return p
}

// solve finds x in [lo, hi] with g(x)=0 for g decreasing, by bisection.
func solve(g func(float64) float64, lo, hi float64) (float64, bool) {
	if g(lo) < 0 || g(hi) > 0 {
		return 0, false
	}
	for i := 0; i < 200 && hi-lo > 1e-15; i++ {
		mid := (lo + hi) / 2
		if g(mid) > 0 {
			lo = mid
		} else {
			hi = mid
		}
	}
	return (lo + hi) / 2, true
}

func yieldFromDirty(flows []CashFlow, dirty float64, f int) (float64, error) {
	y, ok := solve(func(y float64) float64 { return dirtyFromYield(flows, y, f) - dirty }, -0.5, 10)
	if !ok {
		return 0, errInput("price implies a yield outside [-50%%, 1000%%]")
	}
	return y, nil
}

func interpolate(curve []CurvePoint, years float64) float64 {
	days := years * 365
	if days <= float64(curve[0].TenorDays) {
		return curve[0].ZeroRate
	}
	for i := 1; i < len(curve); i++ {
		if days <= float64(curve[i].TenorDays) {
			a, b := curve[i-1], curve[i]
			w := (days - float64(a.TenorDays)) / float64(b.TenorDays-a.TenorDays)
			return a.ZeroRate + w*(b.ZeroRate-a.ZeroRate)
		}
	}
	return curve[len(curve)-1].ZeroRate
}

// zSpread is the constant spread over continuously compounded, linearly
// interpolated zero rates (flat extrapolation) that reprices the flows.
func zSpread(flows []CashFlow, settle time.Time, dirty float64, curve []CurvePoint) (float64, error) {
	c := append([]CurvePoint(nil), curve...)
	sort.Slice(c, func(i, j int) bool { return c[i].TenorDays < c[j].TenorDays })
	g := func(s float64) float64 {
		p := 0.0
		for _, cf := range flows {
			t := actualDays(settle, cf.Date) / 365
			p += cf.Amount * math.Exp(-(interpolate(c, t)+s)*t)
		}
		return p - dirty
	}
	s, ok := solve(g, -1, 5)
	if !ok {
		return 0, errInput("z-spread outside [-100%%, 500%%]")
	}
	return s, nil
}

// AnalyzeBond prices a bond at settlement from exactly one of clean price
// (per 100) or yield to maturity. Durations and convexity are to maturity.
func AnalyzeBond(b Bond, settle time.Time, clean, ytm *float64, curve []CurvePoint) (BondResult, error) {
	if err := b.validate(settle); err != nil {
		return BondResult{}, err
	}
	if (clean == nil) == (ytm == nil) {
		return BondResult{}, errInput("supply exactly one of clean price or yield")
	}
	f := b.compounding()
	acc, prev, next := b.accrued(settle)
	toMaturity := b.flows(settle, b.Maturity, b.Redemption)
	r := BondResult{Accrued: acc, PreviousCoupon: prev, NextCoupon: next, CashFlows: toMaturity}
	if clean != nil {
		if *clean <= 0 {
			return BondResult{}, errInput("clean price must be positive")
		}
		r.CleanPrice, r.DirtyPrice = *clean, *clean+acc
		y, err := yieldFromDirty(toMaturity, r.DirtyPrice, f)
		if err != nil {
			return BondResult{}, err
		}
		r.YieldToMaturity = y
	} else {
		if *ytm <= -float64(f) {
			return BondResult{}, errInput("yield must exceed -compounding frequency")
		}
		r.YieldToMaturity = *ytm
		r.DirtyPrice = dirtyFromYield(toMaturity, *ytm, f)
		r.CleanPrice = r.DirtyPrice - acc
	}
	y, ff := r.YieldToMaturity, float64(f)
	v := 1 / (1 + y/ff)
	var mac, cx float64
	for _, cf := range toMaturity {
		pv := cf.Amount * math.Pow(v, cf.Periods)
		mac += cf.Periods / ff * pv
		cx += cf.Periods * (cf.Periods + 1) * pv
	}
	r.MacaulayDuration = mac / r.DirtyPrice
	r.ModifiedDuration = r.MacaulayDuration * v
	r.Convexity = cx * v * v / (ff * ff * r.DirtyPrice)
	r.DV01 = r.ModifiedDuration * r.DirtyPrice / 10000
	r.YieldToWorst, r.WorkoutDate = r.YieldToMaturity, b.Maturity
	for _, c := range b.Calls {
		if !c.Date.After(settle) || !c.Date.Before(b.Maturity) {
			continue
		}
		yc, err := yieldFromDirty(b.flows(settle, c.Date, c.Price), r.DirtyPrice, f)
		if err != nil {
			return BondResult{}, err
		}
		if yc < r.YieldToWorst {
			r.YieldToWorst, r.WorkoutDate = yc, c.Date
		}
	}
	if len(curve) > 0 {
		s, err := zSpread(toMaturity, settle, r.DirtyPrice, curve)
		if err != nil {
			return BondResult{}, err
		}
		r.ZSpread = &s
	}
	return r, nil
}
