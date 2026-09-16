package repository

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"pricing-platform/internal/db"
	"pricing-platform/internal/domain"
	"time"
)

type Postgres struct{ q *db.Queries }

func New(d db.DBTX) *Postgres  { return &Postgres{db.New(d)} }
func uid(s string) pgtype.UUID { var u pgtype.UUID; _ = u.Scan(s); return u }
func translate(e error) error {
	if errors.Is(e, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}
	var p *pgconn.PgError
	if errors.As(e, &p) && (p.Code == "23503" || p.Code == "23514" || p.Code == "22003") {
		return domain.ErrInvalid
	}
	return e
}
func (p *Postgres) Search(c context.Context, s string) ([]domain.Instrument, error) {
	v, e := p.q.SearchInstruments(c, s)
	out := []domain.Instrument{}
	for _, x := range v {
		out = append(out, domain.Instrument{ID: x.ID, InstrumentType: x.InstrumentType, Currency: x.Currency, Name: x.Name})
	}
	return out, e
}
func (p *Postgres) Resolve(c context.Context, s, v, venue string) ([]domain.Instrument, error) {
	rows, e := p.q.ResolveInstrument(c, db.ResolveInstrumentParams{Scheme: s, Value: v, Venue: venue})
	out := []domain.Instrument{}
	for _, x := range rows {
		out = append(out, domain.Instrument{ID: x.ID, InstrumentType: x.InstrumentType, Currency: x.Currency, Name: x.Name})
	}
	return out, e
}
func (p *Postgres) Quote(c context.Context, id string) (domain.Quote, error) {
	x, e := p.q.LatestQuote(c, uid(id))
	return domain.Quote{InstrumentID: x.InstrumentID, SourceID: x.SourceID, ObservedAt: x.ObservedAt.Time, ReceivedAt: x.ReceivedAt.Time, Bid: x.Bid, Ask: x.Ask}, translate(e)
}
func (p *Postgres) Expirations(c context.Context, id string) ([]string, error) {
	return p.q.Expirations(c, uid(id))
}
func (p *Postgres) Chain(c context.Context, id, date string) ([]domain.Option, error) {
	t, _ := time.Parse("2006-01-02", date)
	rows, e := p.q.OptionChain(c, db.OptionChainParams{ID: uid(id), Expiration: pgtype.Date{Time: t, Valid: true}})
	out := []domain.Option{}
	for _, x := range rows {
		out = append(out, domain.Option{InstrumentID: x.InstrumentID, Expiration: x.Expiration, Strike: x.Strike, RightType: x.RightType, ExerciseStyle: x.ExerciseStyle, Bid: x.Bid, Ask: x.Ask, SourceID: x.SourceID})
	}
	return out, e
}
func (p *Postgres) Curves(c context.Context, currency string) ([]domain.Curve, error) {
	rows, e := p.q.Curves(c, currency)
	out := []domain.Curve{}
	for _, x := range rows {
		out = append(out, domain.Curve{ID: x.ID, Name: x.Name, Currency: x.Currency})
	}
	return out, e
}
func (p *Postgres) Points(c context.Context, id string) ([]domain.CurvePoint, error) {
	rows, e := p.q.CurvePoints(c, uid(id))
	out := []domain.CurvePoint{}
	for _, x := range rows {
		out = append(out, domain.CurvePoint{TenorDays: x.TenorDays, ZeroRate: x.ZeroRate, ObservedAt: x.ObservedAt.Time, SourceID: x.SourceID})
	}
	return out, e
}
func (p *Postgres) IngestQuote(c context.Context, x domain.Quote) error {
	var bid, ask pgtype.Numeric
	if e := bid.Scan(x.Bid); e != nil {
		return domain.ErrInvalid
	}
	if e := ask.Scan(x.Ask); e != nil {
		return domain.ErrInvalid
	}
	return translate(p.q.InsertQuote(c, db.InsertQuoteParams{InstrumentID: uid(x.InstrumentID), SourceID: x.SourceID, ObservedAt: pgtype.Timestamptz{Time: x.ObservedAt, Valid: true}, Bid: bid, Ask: ask}))
}

var _ domain.Store = (*Postgres)(nil)
