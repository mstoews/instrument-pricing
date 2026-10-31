package repository

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"pricing-platform/internal/db"
	"pricing-platform/internal/domain"
	"time"
)

func date(s string) pgtype.Date {
	t, e := time.Parse("2006-01-02", s)
	return pgtype.Date{Time: t, Valid: s != "" && e == nil}
}

// num parses a validated decimal string; empty becomes SQL NULL.
func num(s string) (pgtype.Numeric, error) {
	var n pgtype.Numeric
	if s == "" {
		return n, nil
	}
	if e := n.Scan(s); e != nil {
		return n, domain.ErrInvalid
	}
	return n, nil
}

func (p *Postgres) inTx(c context.Context, fn func(*db.Queries) error) error {
	b, ok := p.conn.(interface {
		Begin(context.Context) (pgx.Tx, error)
	})
	if !ok {
		return errors.New("repository connection does not support transactions")
	}
	tx, e := b.Begin(c)
	if e != nil {
		return e
	}
	defer tx.Rollback(c)
	if e = fn(p.q.WithTx(tx)); e != nil {
		return translate(e)
	}
	return tx.Commit(c)
}

func insertInstrument(c context.Context, q *db.Queries, kind, ccy, name string, ids []domain.Identifier) (string, error) {
	id, e := q.InsertInstrument(c, db.InsertInstrumentParams{InstrumentType: kind, Currency: ccy, Name: name})
	if e != nil {
		return "", e
	}
	for _, x := range ids {
		if e = q.InsertIdentifier(c, db.InsertIdentifierParams{InstrumentID: uid(id), Scheme: x.Scheme, Value: x.Value, Venue: x.Venue}); e != nil {
			return "", e
		}
	}
	return id, nil
}

func (p *Postgres) CreateEquity(c context.Context, x domain.NewEquity) (id string, e error) {
	e = p.inTx(c, func(q *db.Queries) error {
		if id, e = insertInstrument(c, q, "equity", x.Currency, x.Name, x.Identifiers); e != nil {
			return e
		}
		return q.InsertEquity(c, db.InsertEquityParams{InstrumentID: uid(id), Exchange: x.Exchange, Country: x.Country, Sector: x.Sector})
	})
	if e != nil {
		return "", e
	}
	return id, nil
}

func (p *Postgres) CreateBond(c context.Context, x domain.NewBond) (id string, e error) {
	params := db.InsertBondParams{Issuer: x.Issuer, IssueDate: date(x.IssueDate), Maturity: date(x.Maturity), CouponFrequency: int16(*x.CouponFrequency), DayCount: x.DayCount}
	if params.CouponRate, e = num(x.CouponRate); e != nil {
		return "", e
	}
	if params.FaceValue, e = num(x.FaceValue); e != nil {
		return "", e
	}
	if params.Redemption, e = num(x.Redemption); e != nil {
		return "", e
	}
	e = p.inTx(c, func(q *db.Queries) error {
		if id, e = insertInstrument(c, q, "bond", x.Currency, x.Name, x.Identifiers); e != nil {
			return e
		}
		params.InstrumentID = uid(id)
		if e = q.InsertBond(c, params); e != nil {
			return e
		}
		for _, call := range x.Calls {
			price, e := num(call.Price)
			if e != nil {
				return e
			}
			if e = q.InsertBondCall(c, db.InsertBondCallParams{InstrumentID: uid(id), CallDate: date(call.Date), CallPrice: price}); e != nil {
				return e
			}
		}
		return nil
	})
	if e != nil {
		return "", e
	}
	return id, nil
}

func (p *Postgres) Bond(c context.Context, id string) (domain.Bond, error) {
	x, e := p.q.GetBond(c, uid(id))
	if e != nil {
		return domain.Bond{}, translate(e)
	}
	freq := int(x.CouponFrequency)
	b := domain.Bond{Instrument: domain.Instrument{ID: x.ID, InstrumentType: x.InstrumentType, Currency: x.Currency, Name: x.Name},
		BondTerms: domain.BondTerms{Issuer: x.Issuer, IssueDate: x.IssueDate, Maturity: x.Maturity, CouponRate: x.CouponRate, CouponFrequency: &freq,
			DayCount: x.DayCount, FaceValue: x.FaceValue, Redemption: x.Redemption, Calls: []domain.BondCall{}}, Identifiers: []domain.Identifier{}}
	ids, e := p.q.Identifiers(c, uid(id))
	if e != nil {
		return domain.Bond{}, e
	}
	for _, i := range ids {
		b.Identifiers = append(b.Identifiers, domain.Identifier{Scheme: i.Scheme, Value: i.Value, Venue: i.Venue})
	}
	calls, e := p.q.BondCalls(c, uid(id))
	if e != nil {
		return domain.Bond{}, e
	}
	for _, call := range calls {
		b.Calls = append(b.Calls, domain.BondCall{Date: call.CallDate, Price: call.CallPrice})
	}
	return b, nil
}

func (p *Postgres) Bars(c context.Context, id, source, from, to string) ([]domain.PriceBar, error) {
	rows, e := p.q.PriceBars(c, db.PriceBarsParams{ID: uid(id), SourceID: source, FromDate: date(from), ToDate: date(to)})
	out := []domain.PriceBar{}
	for _, x := range rows {
		out = append(out, domain.PriceBar{InstrumentID: x.InstrumentID, SourceID: x.SourceID, Date: x.BarDate, Open: x.Open, High: x.High, Low: x.Low, Close: x.Close, AdjClose: x.AdjClose, Volume: x.Volume})
	}
	return out, e
}

func (p *Postgres) BarOnOrBefore(c context.Context, id, source, on string) (domain.PriceBar, error) {
	x, e := p.q.LatestBarOnOrBefore(c, db.LatestBarOnOrBeforeParams{ID: uid(id), SourceID: source, OnDate: date(on)})
	return domain.PriceBar{InstrumentID: x.InstrumentID, SourceID: x.SourceID, Date: x.BarDate, Open: x.Open, High: x.High, Low: x.Low, Close: x.Close, AdjClose: x.AdjClose, Volume: x.Volume}, translate(e)
}

// IngestBars inserts all bars in one transaction and returns how many were new.
func (p *Postgres) IngestBars(c context.Context, bars []domain.PriceBar) (n int64, e error) {
	e = p.inTx(c, func(q *db.Queries) error {
		for _, b := range bars {
			params := db.InsertPriceBarParams{InstrumentID: uid(b.InstrumentID), SourceID: b.SourceID, BarDate: date(b.Date)}
			for _, f := range []struct {
				target *pgtype.Numeric
				value  string
			}{{&params.Open, b.Open}, {&params.High, b.High}, {&params.Low, b.Low}, {&params.Close, b.Close}, {&params.AdjClose, b.AdjClose}, {&params.Volume, b.Volume}} {
				if *f.target, e = num(f.value); e != nil {
					return e
				}
			}
			k, e := q.InsertPriceBar(c, params)
			if e != nil {
				return e
			}
			n += k
		}
		return nil
	})
	if e != nil {
		return 0, e
	}
	return n, nil
}

var _ domain.SecurityStore = (*Postgres)(nil)
