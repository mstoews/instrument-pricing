// Package analytics holds persistence-independent fixed income and equity
// calculations. Inputs and outputs are float64; callers own decimal-string
// validation and formatting at the transport boundary.
package analytics

import (
	"fmt"
	"time"
)

const (
	Thirty360  = "30/360"
	Act360     = "ACT/360"
	Act365F    = "ACT/365F"
	ActAct     = "ACT/ACT"
	dateLayout = "2006-01-02"
)

func ValidDayCount(dc string) bool {
	return dc == Thirty360 || dc == Act360 || dc == Act365F || dc == ActAct
}

func ParseDate(s string) (time.Time, error) { return time.Parse(dateLayout, s) }

func FormatDate(t time.Time) string { return t.Format(dateLayout) }

func actualDays(a, b time.Time) float64 { return b.Sub(a).Hours() / 24 }

// days30360 is the US (bond basis) 30/360 day count.
func days30360(a, b time.Time) float64 {
	y1, m1, d1 := a.Date()
	y2, m2, d2 := b.Date()
	if d1 == 31 {
		d1 = 30
	}
	if d2 == 31 && d1 == 30 {
		d2 = 30
	}
	return float64(360*(y2-y1) + 30*(int(m2)-int(m1)) + (d2 - d1))
}

// periodFraction is the fraction of a coupon period between a and b, where
// [start, end] is the regular coupon period containing them and freq is the
// number of coupons per year. For ACT/360 and ACT/365F this is the money-market
// fraction scaled to periods, so a full period need not equal exactly 1.
func periodFraction(dc string, a, b, start, end time.Time, freq int) float64 {
	switch dc {
	case Thirty360:
		return days30360(a, b) * float64(freq) / 360
	case Act360:
		return actualDays(a, b) * float64(freq) / 360
	case Act365F:
		return actualDays(a, b) * float64(freq) / 365
	default: // ACT/ACT (ICMA)
		return actualDays(a, b) / actualDays(start, end)
	}
}

// yearFraction is used where there is no coupon period (zero-coupon bonds).
func yearFraction(dc string, a, b time.Time) float64 {
	switch dc {
	case Thirty360:
		return days30360(a, b) / 360
	case Act360:
		return actualDays(a, b) / 360
	case ActAct:
		return actualDays(a, b) / 365.25
	default:
		return actualDays(a, b) / 365
	}
}

// addMonths adds n months, clamping to month end; eom forces the last day of
// the resulting month (end-of-month rule for month-end maturities).
func addMonths(t time.Time, n int, eom bool) time.Time {
	y, m, d := t.Date()
	first := time.Date(y, m+time.Month(n), 1, 0, 0, 0, 0, time.UTC)
	last := first.AddDate(0, 1, -1).Day()
	if eom || d > last {
		d = last
	}
	return time.Date(first.Year(), first.Month(), d, 0, 0, 0, 0, time.UTC)
}

func isMonthEnd(t time.Time) bool { return t.AddDate(0, 0, 1).Day() == 1 }

func errInput(format string, args ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{ErrInput}, args...)...)
}
