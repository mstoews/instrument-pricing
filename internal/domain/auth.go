package domain

import (
	"context"
	"errors"
	"time"
)

var (
	ErrUnauthorized = errors.New("unauthorized")
	ErrForbidden    = errors.New("forbidden")
	ErrRateLimited  = errors.New("too many attempts")
)

// Permission is what a route requires of its caller.
type Permission int

const (
	PermPublic Permission = iota // no credentials
	PermRead                     // any authenticated caller: GET endpoints and calculators
	PermWrite                    // admins and ingest-scoped API keys: ingestion and security master
	PermAdmin                    // admin users signed in with a session, never an API key
)

const (
	RoleUser    = "user"
	RoleAdmin   = "admin"
	ScopeRead   = "read"
	ScopeIngest = "ingest"
)

// Principal is the authenticated caller: a user session or an API key.
type Principal struct {
	UserID   string
	Email    string
	Role     string // users only
	Scope    string // API keys only
	APIKeyID string // set for API keys
	FamilyID string // set for sessions; logout revokes the family
}

func (p Principal) Allows(perm Permission) bool {
	switch perm {
	case PermPublic, PermRead:
		return true
	case PermWrite:
		return (p.APIKeyID == "" && p.Role == RoleAdmin) || (p.APIKeyID != "" && p.Scope == ScopeIngest)
	case PermAdmin:
		return p.APIKeyID == "" && p.Role == RoleAdmin
	}
	return false
}

type User struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Role  string `json:"role"`
}

type Credentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type TokenPair struct {
	TokenType        string    `json:"token_type"`
	AccessToken      string    `json:"access_token"`
	AccessExpiresAt  time.Time `json:"access_expires_at"`
	RefreshToken     string    `json:"refresh_token"`
	RefreshExpiresAt time.Time `json:"refresh_expires_at"`
	User             User      `json:"user"`
}

type APIKey struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Scope      string     `json:"scope"`
	Owner      string     `json:"owner"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
	RevokedAt  *time.Time `json:"revoked_at"`
	Key        string     `json:"key,omitempty"` // only in the creation response
}

type StoredUser struct {
	User
	PasswordHash string
	Disabled     bool
}

type NewSession struct {
	UserID, FamilyID              string
	AccessHash, RefreshHash       []byte
	AccessExpires, RefreshExpires time.Time
	UserAgent                     string
}

// RefreshState is the locked session row a refresh token resolved to.
type RefreshState struct {
	SessionID, FamilyID, UserID, UserAgent string
	Used, Expired, Disabled                bool
}

type AuthStore interface {
	CreateUser(ctx context.Context, email, passwordHash, role string) (string, error)
	UpsertUser(ctx context.Context, email, passwordHash, role string) (string, error)
	UserByEmail(context.Context, string) (StoredUser, error)
	UserByID(context.Context, string) (StoredUser, error)
	CreateSession(context.Context, NewSession) error
	SessionByAccess(context.Context, []byte) (Principal, error)
	// RotateRefresh locks the session for refreshHash (ErrUnauthorized if
	// unknown) and, in one transaction, revokes its family and/or rotates it
	// to the successor that decide returns.
	RotateRefresh(ctx context.Context, refreshHash []byte, decide func(RefreshState) (next *NewSession, revokeFamily bool)) error
	RevokeFamily(context.Context, string) error
	FamilyByRefresh(context.Context, []byte) (string, error)
	CreateAPIKey(ctx context.Context, userID, name, scope string, hash []byte) (APIKey, error)
	APIKeyByHash(context.Context, []byte) (Principal, *time.Time, error)
	TouchAPIKey(context.Context, string) error
	APIKeys(context.Context) ([]APIKey, error)
	RevokeAPIKey(context.Context, string) error
}
