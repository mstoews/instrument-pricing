package domain

import (
	"context"
	"errors"
	"time"
)

var ErrInvalid = errors.New("invalid request")
var ErrNotFound = errors.New("not found")
var ErrConflict = errors.New("conflict")

type Instrument struct {
	ID             string `json:"id"`
	InstrumentType string `json:"instrument_type"`
	Currency       string `json:"currency"`
	Name           string `json:"name"`
}
type Quote struct {
	InstrumentID string    `json:"instrument_id"`
	SourceID     string    `json:"source_id"`
	ObservedAt   time.Time `json:"observed_at"`
	ReceivedAt   time.Time `json:"received_at"`
	Bid          string    `json:"bid"`
	Ask          string    `json:"ask"`
}
type Option struct {
	InstrumentID  string `json:"instrument_id"`
	Expiration    string `json:"expiration"`
	Strike        string `json:"strike"`
	RightType     string `json:"right_type"`
	ExerciseStyle string `json:"exercise_style"`
	Bid           string `json:"bid"`
	Ask           string `json:"ask"`
	SourceID      string `json:"source_id"`
}
type Curve struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Currency string `json:"currency"`
}
type CurvePoint struct {
	TenorDays  int32     `json:"tenor_days"`
	ZeroRate   string    `json:"zero_rate"`
	ObservedAt time.Time `json:"observed_at"`
	SourceID   string    `json:"source_id"`
}

// CurveSnapshot is one observation of a zero curve: continuously compounded
// decimal zero rates by tenor in days from the observation.
type CurveSnapshot struct {
	CurveName  string      `json:"curve_name"`
	Currency   string      `json:"currency"`
	SourceID   string      `json:"source_id"`
	ObservedAt time.Time   `json:"observed_at"`
	Points     []ZeroPoint `json:"points"`
}
type CurveIngestResult struct {
	CurveID  string `json:"curve_id"`
	Inserted bool   `json:"inserted"` // false when this curve/source/time was already stored
}
type Store interface {
	Search(context.Context, string) ([]Instrument, error)
	Resolve(context.Context, string, string, string) ([]Instrument, error)
	Quote(context.Context, string) (Quote, error)
	Expirations(context.Context, string) ([]string, error)
	Chain(context.Context, string, string) ([]Option, error)
	Curves(context.Context, string) ([]Curve, error)
	Points(context.Context, string) ([]CurvePoint, error)
	IngestQuote(context.Context, Quote) error
	IngestCurve(context.Context, CurveSnapshot) (CurveIngestResult, error)
}
