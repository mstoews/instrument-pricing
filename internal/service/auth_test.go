package service

import (
	"context"
	"errors"
	"pricing-platform/internal/domain"
	"sync"
	"testing"
	"time"
)

type memSession struct {
	domain.NewSession
	id               string
	rotated, revoked bool
}

// memAuth is an in-memory domain.AuthStore with the same semantics as Postgres.
type memAuth struct {
	mu       sync.Mutex
	users    map[string]domain.StoredUser
	sessions []*memSession
	keys     map[string]domain.APIKey
	keyHash  map[string]string
	touches  int
}

func newMemAuth() *memAuth {
	return &memAuth{users: map[string]domain.StoredUser{}, keys: map[string]domain.APIKey{}, keyHash: map[string]string{}}
}

func (m *memAuth) CreateUser(_ context.Context, e, h, role string) (string, error) {
	if _, ok := m.users[e]; ok {
		return "", domain.ErrConflict
	}
	return m.UpsertUser(context.Background(), e, h, role)
}
func (m *memAuth) UpsertUser(_ context.Context, e, h, role string) (string, error) {
	u := m.users[e]
	if u.ID == "" {
		u.ID = newUUID()
	}
	u.Email, u.PasswordHash, u.Role = e, h, role
	m.users[e] = u
	return u.ID, nil
}
func (m *memAuth) UserByEmail(_ context.Context, e string) (domain.StoredUser, error) {
	u, ok := m.users[e]
	if !ok {
		return u, domain.ErrNotFound
	}
	return u, nil
}
func (m *memAuth) UserByID(_ context.Context, id string) (domain.StoredUser, error) {
	for _, u := range m.users {
		if u.ID == id {
			return u, nil
		}
	}
	return domain.StoredUser{}, domain.ErrNotFound
}
func (m *memAuth) CreateSession(_ context.Context, s domain.NewSession) error {
	m.sessions = append(m.sessions, &memSession{NewSession: s, id: newUUID()})
	return nil
}
func (m *memAuth) SessionByAccess(_ context.Context, h []byte) (domain.Principal, error) {
	for _, s := range m.sessions {
		if string(s.AccessHash) == string(h) && !s.rotated && !s.revoked && time.Now().Before(s.AccessExpires) {
			u, _ := m.UserByID(context.Background(), s.UserID)
			if !u.Disabled {
				return domain.Principal{UserID: u.ID, Email: u.Email, Role: u.Role, FamilyID: s.FamilyID}, nil
			}
		}
	}
	return domain.Principal{}, domain.ErrNotFound
}
func (m *memAuth) RotateRefresh(_ context.Context, h []byte, decide func(domain.RefreshState) (*domain.NewSession, bool)) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, s := range m.sessions {
		if string(s.RefreshHash) == string(h) {
			u, _ := m.UserByID(context.Background(), s.UserID)
			next, revoke := decide(domain.RefreshState{SessionID: s.id, FamilyID: s.FamilyID, UserID: s.UserID,
				Used: s.rotated || s.revoked, Expired: !time.Now().Before(s.RefreshExpires), Disabled: u.Disabled})
			if revoke {
				_ = m.RevokeFamily(context.Background(), s.FamilyID)
			}
			if next != nil {
				s.rotated = true
				return m.CreateSession(context.Background(), *next)
			}
			return nil
		}
	}
	return domain.ErrUnauthorized
}
func (m *memAuth) RevokeFamily(_ context.Context, f string) error {
	for _, s := range m.sessions {
		if s.FamilyID == f {
			s.revoked = true
		}
	}
	return nil
}
func (m *memAuth) FamilyByRefresh(_ context.Context, h []byte) (string, error) {
	for _, s := range m.sessions {
		if string(s.RefreshHash) == string(h) {
			return s.FamilyID, nil
		}
	}
	return "", domain.ErrNotFound
}
func (m *memAuth) CreateAPIKey(_ context.Context, userID, name, scope string, h []byte) (domain.APIKey, error) {
	k := domain.APIKey{ID: newUUID(), Name: name, Scope: scope, Owner: userID}
	m.keys[k.ID], m.keyHash[string(h)] = k, k.ID
	return k, nil
}
func (m *memAuth) APIKeyByHash(_ context.Context, h []byte) (domain.Principal, *time.Time, error) {
	k, ok := m.keys[m.keyHash[string(h)]]
	if !ok || k.RevokedAt != nil {
		return domain.Principal{}, nil, domain.ErrNotFound
	}
	return domain.Principal{UserID: k.Owner, Scope: k.Scope, APIKeyID: k.ID}, k.LastUsedAt, nil
}
func (m *memAuth) TouchAPIKey(_ context.Context, id string) error {
	k := m.keys[id]
	now := time.Now()
	k.LastUsedAt = &now
	m.keys[id] = k
	m.touches++
	return nil
}
func (m *memAuth) APIKeys(context.Context) ([]domain.APIKey, error) { return nil, nil }
func (m *memAuth) RevokeAPIKey(_ context.Context, id string) error {
	k, ok := m.keys[id]
	if !ok {
		return domain.ErrNotFound
	}
	now := time.Now()
	k.RevokedAt = &now
	m.keys[id] = k
	return nil
}

const pw = "a long enough password"

func setup(t *testing.T) (*Auth, *memAuth) {
	t.Helper()
	m := newMemAuth()
	a := NewAuth(m, AuthConfig{AllowSignup: true})
	if _, err := a.SetUser(context.Background(), " Admin@Example.com ", pw, domain.RoleAdmin); err != nil {
		t.Fatal(err)
	}
	return a, m
}

func TestLoginRefreshRotationAndReuse(t *testing.T) {
	a, _ := setup(t)
	c := context.Background()
	first, err := a.Login(c, domain.Credentials{Email: "admin@EXAMPLE.com", Password: pw}, "test")
	if err != nil || first.User.Role != domain.RoleAdmin || first.TokenType != "Bearer" {
		t.Fatalf("%+v %v", first, err)
	}
	if p, err := a.Authenticate(c, first.AccessToken); err != nil || !p.Allows(domain.PermAdmin) {
		t.Fatalf("access token: %v", err)
	}
	second, err := a.Refresh(c, first.RefreshToken, "")
	if err != nil || second.RefreshToken == first.RefreshToken {
		t.Fatalf("refresh: %v", err)
	}
	if _, err := a.Authenticate(c, first.AccessToken); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatal("rotated access token still valid")
	}
	// Replaying the first refresh token is theft: the whole family dies.
	if _, err := a.Refresh(c, first.RefreshToken, ""); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatal("refresh token reuse accepted")
	}
	if _, err := a.Authenticate(c, second.AccessToken); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatal("family not revoked after reuse")
	}
	if _, err := a.Refresh(c, second.RefreshToken, ""); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatal("successor refresh survived family revocation")
	}
}

func TestLogoutRevokesOnlyThatLogin(t *testing.T) {
	a, _ := setup(t)
	c := context.Background()
	phone, _ := a.Login(c, domain.Credentials{Email: "admin@example.com", Password: pw}, "ios")
	web, _ := a.Login(c, domain.Credentials{Email: "admin@example.com", Password: pw}, "web")
	// Logout works with the refresh token even after the access token was rotated away.
	rotated, err := a.Refresh(c, phone.RefreshToken, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Logout(c, rotated.RefreshToken); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Authenticate(c, rotated.AccessToken); err == nil {
		t.Fatal("logged-out token valid")
	}
	for _, tok := range []string{"", "nonsense", rotated.RefreshToken} {
		if err := a.Logout(c, tok); err != nil {
			t.Fatalf("logout(%q) should succeed silently: %v", tok, err)
		}
	}
	if _, err := a.Authenticate(c, phone.AccessToken); err == nil {
		t.Fatal("logged-out token valid")
	}
	if _, err := a.Authenticate(c, web.AccessToken); err != nil {
		t.Fatal("logout affected another login")
	}
}

func TestLoginFailuresAndRateLimit(t *testing.T) {
	a, m := setup(t)
	c := context.Background()
	for _, cr := range []domain.Credentials{{Email: "admin@example.com", Password: "wrong password!!"}, {Email: "nobody@example.com", Password: pw}, {Email: "not-an-email", Password: pw}} {
		if _, err := a.Login(c, cr, ""); !errors.Is(err, domain.ErrUnauthorized) || err.Error() != "unauthorized: invalid email or password" {
			t.Errorf("%s: %v", cr.Email, err)
		}
	}
	u := m.users["admin@example.com"]
	u.Disabled = true
	m.users["admin@example.com"] = u
	if _, err := a.Login(c, domain.Credentials{Email: "admin@example.com", Password: pw}, ""); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatal("disabled user logged in")
	}
	u.Disabled = false
	m.users["admin@example.com"] = u
	for i := 0; i < 10; i++ {
		_, _ = a.Login(c, domain.Credentials{Email: "admin@example.com", Password: "wrong password!!"}, "")
	}
	if _, err := a.Login(c, domain.Credentials{Email: "admin@example.com", Password: pw}, ""); !errors.Is(err, domain.ErrRateLimited) {
		t.Fatalf("not rate limited: %v", err)
	}
}

func TestRegisterAndAPIKeys(t *testing.T) {
	a, m := setup(t)
	c := context.Background()
	if _, err := a.Register(c, domain.Credentials{Email: "u@example.com", Password: "short"}, "ip"); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal("short password accepted")
	}
	u, err := a.Register(c, domain.Credentials{Email: "U@example.com", Password: pw}, "ip")
	if err != nil || u.Role != domain.RoleUser || u.Email != "u@example.com" {
		t.Fatalf("%+v %v", u, err)
	}
	if _, err := a.Register(c, domain.Credentials{Email: "u@example.com", Password: pw}, "ip"); !errors.Is(err, domain.ErrConflict) {
		t.Fatal("duplicate account")
	}
	closed := NewAuth(m, AuthConfig{})
	if _, err := closed.Register(c, domain.Credentials{Email: "v@example.com", Password: pw}, "ip"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatal("sign-up not disabled by default")
	}
	k, err := a.IssueAPIKey(c, "admin@example.com", "ingestion", domain.ScopeIngest)
	if err != nil || k.Key == "" {
		t.Fatal(err)
	}
	p, err := a.Authenticate(c, k.Key)
	if err != nil || !p.Allows(domain.PermWrite) || p.Allows(domain.PermAdmin) {
		t.Fatalf("api key principal %+v %v", p, err)
	}
	_, _ = a.Authenticate(c, k.Key)
	if m.touches != 1 {
		t.Fatalf("last_used_at written %d times", m.touches)
	}
	if err := a.RevokeAPIKey(c, k.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Authenticate(c, k.Key); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatal("revoked key accepted")
	}
	if _, err := a.IssueAPIKey(c, "admin@example.com", "x", "admin"); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal("bad scope accepted")
	}
	for _, tok := range []string{"", "ipa_short", "Bearer x", "ipr_" + string(make([]byte, 43))} {
		if _, err := a.Authenticate(c, tok); !errors.Is(err, domain.ErrUnauthorized) {
			t.Errorf("accepted %q", tok)
		}
	}
}

func TestPermissions(t *testing.T) {
	admin := domain.Principal{Role: domain.RoleAdmin}
	user := domain.Principal{Role: domain.RoleUser}
	ingest := domain.Principal{APIKeyID: "k", Scope: domain.ScopeIngest, Role: domain.RoleAdmin}
	read := domain.Principal{APIKeyID: "k", Scope: domain.ScopeRead}
	for _, tc := range []struct {
		p    domain.Principal
		perm domain.Permission
		want bool
	}{
		{user, domain.PermRead, true}, {user, domain.PermWrite, false}, {user, domain.PermAdmin, false},
		{admin, domain.PermWrite, true}, {admin, domain.PermAdmin, true},
		{ingest, domain.PermWrite, true}, {ingest, domain.PermAdmin, false},
		{read, domain.PermRead, true}, {read, domain.PermWrite, false},
	} {
		if got := tc.p.Allows(tc.perm); got != tc.want {
			t.Errorf("%+v %d: got %v", tc.p, tc.perm, got)
		}
	}
}
