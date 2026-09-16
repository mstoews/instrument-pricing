package service

import (
	"context"
	"math/big"
	"pricing-platform/internal/domain"
	"regexp"
	"strings"
	"time"
)

type Market struct{ store domain.Store }

func New(s domain.Store) *Market { return &Market{s} }

var uuid = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
var decimal = regexp.MustCompile(`^[0-9]{1,12}(\.[0-9]{1,8})?$`)
var currency = regexp.MustCompile(`^[A-Z]{3}$`)

func (m *Market) Search(c context.Context, s string) ([]domain.Instrument, error) {
	s = strings.TrimSpace(s)
	if len(s) > 100 {
		return nil, domain.ErrInvalid
	}
	return m.store.Search(c, s)
}
func (m *Market) Resolve(c context.Context, s, v, venue string) ([]domain.Instrument, error) {
	if s == "" || v == "" || len(v) > 100 || len(s) > 30 || len(venue) > 30 {
		return nil, domain.ErrInvalid
	}
	return m.store.Resolve(c, s, v, venue)
}
func (m *Market) Quote(c context.Context, id string) (domain.Quote, error) {
	if !uuid.MatchString(id) {
		return domain.Quote{}, domain.ErrInvalid
	}
	return m.store.Quote(c, id)
}
func (m *Market) Expirations(c context.Context, id string) ([]string, error) {
	if !uuid.MatchString(id) {
		return nil, domain.ErrInvalid
	}
	return m.store.Expirations(c, id)
}
func (m *Market) Chain(c context.Context, id, date string) ([]domain.Option, error) {
	if _, e := time.Parse("2006-01-02", date); e != nil || !uuid.MatchString(id) {
		return nil, domain.ErrInvalid
	}
	return m.store.Chain(c, id, date)
}
func (m *Market) Curves(c context.Context, s string) ([]domain.Curve, error) {
	if !currency.MatchString(s) {
		return nil, domain.ErrInvalid
	}
	return m.store.Curves(c, s)
}
func (m *Market) Points(c context.Context, id string) ([]domain.CurvePoint, error) {
	if !uuid.MatchString(id) {
		return nil, domain.ErrInvalid
	}
	return m.store.Points(c, id)
}
func (m *Market) IngestQuote(c context.Context, q domain.Quote) error {
	if !uuid.MatchString(q.InstrumentID) || q.SourceID == "" || len(q.SourceID) > 50 || q.ObservedAt.IsZero() || !q.ReceivedAt.IsZero() || !decimal.MatchString(q.Bid) || !decimal.MatchString(q.Ask) {
		return domain.ErrInvalid
	}
	b, _ := new(big.Rat).SetString(q.Bid)
	a, _ := new(big.Rat).SetString(q.Ask)
	if a.Cmp(b) < 0 {
		return domain.ErrInvalid
	}
	return m.store.IngestQuote(c, q)
}
