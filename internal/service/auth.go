package service

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"pricing-platform/internal/auth"
	"pricing-platform/internal/domain"
	"regexp"
	"strings"
	"time"
)

type AuthConfig struct {
	AccessTTL, RefreshTTL time.Duration
	AllowSignup           bool
}

// Auth issues and verifies credentials. Access tokens are short-lived;
// refresh tokens rotate on every use, and replaying a rotated refresh token
// revokes every session descended from the same login.
type Auth struct {
	store   domain.AuthStore
	cfg     AuthConfig
	now     func() time.Time
	logins  *auth.Limiter
	signups *auth.Limiter
}

func NewAuth(s domain.AuthStore, cfg AuthConfig) *Auth {
	if cfg.AccessTTL <= 0 {
		cfg.AccessTTL = 15 * time.Minute
	}
	if cfg.RefreshTTL <= 0 {
		cfg.RefreshTTL = 30 * 24 * time.Hour
	}
	return &Auth{store: s, cfg: cfg, now: time.Now,
		logins:  auth.NewLimiter(10, 15*time.Minute),
		signups: auth.NewLimiter(5, time.Hour)}
}

const (
	minPassword = 12
	maxPassword = 128
	apiKeyTouch = 5 * time.Minute
)

var email = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

func normalizeEmail(e string) (string, error) {
	e = strings.ToLower(strings.TrimSpace(e))
	if len(e) > 254 || !email.MatchString(e) {
		return "", invalid("a valid email address is required")
	}
	return e, nil
}

func validPassword(p string) error {
	if n := len([]rune(p)); n < minPassword || n > maxPassword {
		return invalid("password must be %d-%d characters", minPassword, maxPassword)
	}
	return nil
}

func unauthorized(msg string) error { return fmt.Errorf("%w: %s", domain.ErrUnauthorized, msg) }

// SetUser creates or updates a user; used by the operator CLI.
func (a *Auth) SetUser(c context.Context, emailAddr, password, role string) (string, error) {
	e, err := normalizeEmail(emailAddr)
	if err != nil {
		return "", err
	}
	if role != domain.RoleUser && role != domain.RoleAdmin {
		return "", invalid("role must be user or admin")
	}
	if err := validPassword(password); err != nil {
		return "", err
	}
	h, err := auth.HashPassword(password)
	if err != nil {
		return "", err
	}
	return a.store.UpsertUser(c, e, h, role)
}

// Register self-service creates a user-role account when sign-up is enabled.
func (a *Auth) Register(c context.Context, cr domain.Credentials, remote string) (domain.User, error) {
	if !a.cfg.AllowSignup {
		return domain.User{}, fmt.Errorf("%w: sign-up is disabled", domain.ErrForbidden)
	}
	e, err := normalizeEmail(cr.Email)
	if err != nil {
		return domain.User{}, err
	}
	if err := validPassword(cr.Password); err != nil {
		return domain.User{}, err
	}
	if a.signups.Blocked(remote) {
		return domain.User{}, domain.ErrRateLimited
	}
	a.signups.Fail(remote) // every attempt counts against the source address
	h, err := auth.HashPassword(cr.Password)
	if err != nil {
		return domain.User{}, err
	}
	id, err := a.store.CreateUser(c, e, h, domain.RoleUser)
	if err != nil {
		return domain.User{}, err
	}
	return domain.User{ID: id, Email: e, Role: domain.RoleUser}, nil
}

func (a *Auth) issue(userID, familyID, userAgent string) (domain.NewSession, domain.TokenPair, error) {
	access, ah, err := auth.NewToken(auth.AccessPrefix)
	if err != nil {
		return domain.NewSession{}, domain.TokenPair{}, err
	}
	refresh, rh, err := auth.NewToken(auth.RefreshPrefix)
	if err != nil {
		return domain.NewSession{}, domain.TokenPair{}, err
	}
	now := a.now().UTC().Truncate(time.Second)
	if len(userAgent) > 200 {
		userAgent = userAgent[:200]
	}
	s := domain.NewSession{UserID: userID, FamilyID: familyID, AccessHash: ah, RefreshHash: rh,
		AccessExpires: now.Add(a.cfg.AccessTTL), RefreshExpires: now.Add(a.cfg.RefreshTTL), UserAgent: userAgent}
	return s, domain.TokenPair{TokenType: "Bearer", AccessToken: access, AccessExpiresAt: s.AccessExpires, RefreshToken: refresh, RefreshExpiresAt: s.RefreshExpires}, nil
}

func newUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6], b[8] = b[6]&0x0f|0x40, b[8]&0x3f|0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// Login verifies a password and starts a new session family. Failures are
// limited per email; unknown accounts cost the same as wrong passwords.
func (a *Auth) Login(c context.Context, cr domain.Credentials, userAgent string) (domain.TokenPair, error) {
	e, err := normalizeEmail(cr.Email)
	if err != nil || len(cr.Password) > 4*maxPassword {
		return domain.TokenPair{}, unauthorized("invalid email or password")
	}
	if a.logins.Blocked(e) {
		return domain.TokenPair{}, domain.ErrRateLimited
	}
	u, err := a.store.UserByEmail(c, e)
	if errors.Is(err, domain.ErrNotFound) {
		auth.BurnPasswordCheck(cr.Password)
		a.logins.Fail(e)
		return domain.TokenPair{}, unauthorized("invalid email or password")
	}
	if err != nil {
		return domain.TokenPair{}, err
	}
	ok, err := auth.VerifyPassword(u.PasswordHash, cr.Password)
	if err != nil {
		return domain.TokenPair{}, err
	}
	if !ok || u.Disabled {
		a.logins.Fail(e)
		return domain.TokenPair{}, unauthorized("invalid email or password")
	}
	a.logins.Reset(e)
	s, pair, err := a.issue(u.ID, newUUID(), userAgent)
	if err != nil {
		return domain.TokenPair{}, err
	}
	if err := a.store.CreateSession(c, s); err != nil {
		return domain.TokenPair{}, err
	}
	pair.User = u.User
	return pair, nil
}

// Refresh exchanges a refresh token for a new pair. Clients must serialize
// refreshes: a second use of the same token is treated as theft.
func (a *Auth) Refresh(c context.Context, token, userAgent string) (domain.TokenPair, error) {
	if !auth.WellFormed(token, auth.RefreshPrefix) {
		return domain.TokenPair{}, unauthorized("invalid refresh token")
	}
	var pair domain.TokenPair
	var issueErr error
	var reused bool
	var userID string
	err := a.store.RotateRefresh(c, auth.Digest(token), func(st domain.RefreshState) (*domain.NewSession, bool) {
		if st.Used {
			reused = true
			return nil, true
		}
		if st.Expired || st.Disabled {
			return nil, true
		}
		if userAgent == "" {
			userAgent = st.UserAgent
		}
		var s domain.NewSession
		s, pair, issueErr = a.issue(st.UserID, st.FamilyID, userAgent)
		if issueErr != nil {
			return nil, false
		}
		userID = st.UserID
		return &s, false
	})
	if err != nil {
		return domain.TokenPair{}, err
	}
	if issueErr != nil {
		return domain.TokenPair{}, issueErr
	}
	if reused {
		slog.Warn("[auth] refresh token reuse; session family revoked")
	}
	if pair.AccessToken == "" {
		return domain.TokenPair{}, unauthorized("invalid refresh token")
	}
	u, err := a.store.UserByID(c, userID)
	if err != nil {
		return domain.TokenPair{}, err
	}
	pair.User = u.User
	return pair, nil
}

// Logout revokes the session family of a refresh token: that login and every
// token refreshed from it; other logins (other devices) stay signed in. It
// takes the refresh token, which clients always hold, so it works after the
// access token has expired or been rotated by another tab. Unknown tokens
// succeed silently, so the response reveals nothing.
func (a *Auth) Logout(c context.Context, refreshToken string) error {
	if !auth.WellFormed(refreshToken, auth.RefreshPrefix) {
		return nil
	}
	family, err := a.store.FamilyByRefresh(c, auth.Digest(refreshToken))
	if errors.Is(err, domain.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return a.store.RevokeFamily(c, family)
}

// Authenticate resolves a bearer credential to a principal.
func (a *Auth) Authenticate(c context.Context, token string) (domain.Principal, error) {
	switch {
	case auth.WellFormed(token, auth.AccessPrefix):
		p, err := a.store.SessionByAccess(c, auth.Digest(token))
		if errors.Is(err, domain.ErrNotFound) {
			return p, unauthorized("access token is invalid or expired")
		}
		return p, err
	case auth.WellFormed(token, auth.APIKeyPrefix):
		p, last, err := a.store.APIKeyByHash(c, auth.Digest(token))
		if errors.Is(err, domain.ErrNotFound) {
			return p, unauthorized("API key is invalid or revoked")
		}
		if err != nil {
			return p, err
		}
		if last == nil || a.now().Sub(*last) > apiKeyTouch {
			if err := a.store.TouchAPIKey(c, p.APIKeyID); err != nil {
				slog.Warn("[auth] api key last_used_at update failed", "key_id", p.APIKeyID, "error", err)
			}
		}
		return p, nil
	}
	return domain.Principal{}, unauthorized("unrecognized bearer token")
}

func (a *Auth) issueKey(c context.Context, userID, name, scope string) (domain.APIKey, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 100 {
		return domain.APIKey{}, invalid("name must be 1-100 characters")
	}
	if scope != domain.ScopeRead && scope != domain.ScopeIngest {
		return domain.APIKey{}, invalid("scope must be read or ingest")
	}
	key, h, err := auth.NewToken(auth.APIKeyPrefix)
	if err != nil {
		return domain.APIKey{}, err
	}
	k, err := a.store.CreateAPIKey(c, userID, name, scope, h)
	k.Key = key
	return k, err
}

// CreateAPIKey issues a key owned by the calling admin; the key is shown once.
func (a *Auth) CreateAPIKey(c context.Context, p domain.Principal, name, scope string) (domain.APIKey, error) {
	return a.issueKey(c, p.UserID, name, scope)
}

// IssueAPIKey is the operator CLI path: a key owned by an existing user.
func (a *Auth) IssueAPIKey(c context.Context, owner, name, scope string) (domain.APIKey, error) {
	e, err := normalizeEmail(owner)
	if err != nil {
		return domain.APIKey{}, err
	}
	u, err := a.store.UserByEmail(c, e)
	if err != nil {
		return domain.APIKey{}, err
	}
	return a.issueKey(c, u.ID, name, scope)
}

func (a *Auth) APIKeys(c context.Context) ([]domain.APIKey, error) { return a.store.APIKeys(c) }

func (a *Auth) RevokeAPIKey(c context.Context, id string) error {
	if !uuid.MatchString(id) {
		return domain.ErrInvalid
	}
	return a.store.RevokeAPIKey(c, id)
}
