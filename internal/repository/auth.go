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

func ts(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }

func optTime(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	return &t.Time
}

func (p *Postgres) CreateUser(c context.Context, email, hash, role string) (string, error) {
	id, e := p.q.InsertUser(c, db.InsertUserParams{Email: email, PasswordHash: hash, Role: role})
	return id, translate(e)
}

func (p *Postgres) UpsertUser(c context.Context, email, hash, role string) (string, error) {
	id, e := p.q.UpsertUser(c, db.UpsertUserParams{Email: email, PasswordHash: hash, Role: role})
	return id, translate(e)
}

func (p *Postgres) UserByEmail(c context.Context, email string) (domain.StoredUser, error) {
	x, e := p.q.UserByEmail(c, email)
	return domain.StoredUser{User: domain.User{ID: x.ID, Email: x.Email, Role: x.Role}, PasswordHash: x.PasswordHash, Disabled: x.Disabled}, translate(e)
}

func (p *Postgres) UserByID(c context.Context, id string) (domain.StoredUser, error) {
	x, e := p.q.UserByID(c, uid(id))
	return domain.StoredUser{User: domain.User{ID: x.ID, Email: x.Email, Role: x.Role}, Disabled: x.Disabled}, translate(e)
}

func insertSession(c context.Context, q *db.Queries, s domain.NewSession) error {
	return q.InsertSession(c, db.InsertSessionParams{UserID: uid(s.UserID), FamilyID: uid(s.FamilyID), AccessHash: s.AccessHash, RefreshHash: s.RefreshHash,
		AccessExpiresAt: ts(s.AccessExpires), RefreshExpiresAt: ts(s.RefreshExpires), UserAgent: s.UserAgent})
}

func (p *Postgres) CreateSession(c context.Context, s domain.NewSession) error {
	return translate(insertSession(c, p.q, s))
}

func (p *Postgres) SessionByAccess(c context.Context, h []byte) (domain.Principal, error) {
	x, e := p.q.SessionByAccess(c, h)
	return domain.Principal{UserID: x.UserID, Email: x.Email, Role: x.Role, FamilyID: x.FamilyID}, translate(e)
}

func (p *Postgres) RotateRefresh(c context.Context, h []byte, decide func(domain.RefreshState) (*domain.NewSession, bool)) error {
	return p.inTx(c, func(q *db.Queries) error {
		x, e := q.SessionByRefreshForUpdate(c, h)
		if errors.Is(e, pgx.ErrNoRows) {
			return domain.ErrUnauthorized
		}
		if e != nil {
			return e
		}
		next, revoke := decide(domain.RefreshState{SessionID: x.SessionID, FamilyID: x.FamilyID, UserID: x.UserID, UserAgent: x.UserAgent, Used: x.Used, Expired: x.Expired, Disabled: x.Disabled})
		if revoke {
			if e = q.RevokeFamily(c, uid(x.FamilyID)); e != nil {
				return e
			}
		}
		if next == nil {
			return nil
		}
		if e = q.RotateSession(c, uid(x.SessionID)); e != nil {
			return e
		}
		return insertSession(c, q, *next)
	})
}

func (p *Postgres) RevokeFamily(c context.Context, family string) error {
	return translate(p.q.RevokeFamily(c, uid(family)))
}

func (p *Postgres) CreateAPIKey(c context.Context, userID, name, scope string, h []byte) (domain.APIKey, error) {
	x, e := p.q.InsertAPIKey(c, db.InsertAPIKeyParams{UserID: uid(userID), Name: name, Scope: scope, KeyHash: h})
	if e != nil {
		return domain.APIKey{}, translate(e)
	}
	u, e := p.q.UserByID(c, uid(userID))
	return domain.APIKey{ID: x.ID, Name: name, Scope: scope, Owner: u.Email, CreatedAt: x.CreatedAt.Time}, translate(e)
}

func (p *Postgres) APIKeyByHash(c context.Context, h []byte) (domain.Principal, *time.Time, error) {
	x, e := p.q.APIKeyByHash(c, h)
	return domain.Principal{UserID: x.UserID, Email: x.Email, Scope: x.Scope, APIKeyID: x.ID}, optTime(x.LastUsedAt), translate(e)
}

func (p *Postgres) TouchAPIKey(c context.Context, id string) error {
	return translate(p.q.TouchAPIKey(c, uid(id)))
}

func (p *Postgres) APIKeys(c context.Context) ([]domain.APIKey, error) {
	rows, e := p.q.ListAPIKeys(c)
	out := []domain.APIKey{}
	for _, x := range rows {
		out = append(out, domain.APIKey{ID: x.ID, Name: x.Name, Scope: x.Scope, Owner: x.Owner, CreatedAt: x.CreatedAt.Time, LastUsedAt: optTime(x.LastUsedAt), RevokedAt: optTime(x.RevokedAt)})
	}
	return out, e
}

func (p *Postgres) RevokeAPIKey(c context.Context, id string) error {
	n, e := p.q.RevokeAPIKey(c, uid(id))
	if e == nil && n == 0 {
		return domain.ErrNotFound
	}
	return translate(e)
}

var _ domain.AuthStore = (*Postgres)(nil)
