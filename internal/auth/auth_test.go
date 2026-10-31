package auth

import (
	"strings"
	"testing"
	"time"
)

func TestPasswordHash(t *testing.T) {
	h, err := HashPassword("correct horse battery staple")
	if err != nil || !strings.HasPrefix(h, "$argon2id$v=19$m=19456,t=2,p=1$") {
		t.Fatalf("%s %v", h, err)
	}
	if ok, err := VerifyPassword(h, "correct horse battery staple"); !ok || err != nil {
		t.Fatal("valid password rejected")
	}
	if ok, _ := VerifyPassword(h, "correct horse battery stapl"); ok {
		t.Fatal("wrong password accepted")
	}
	other, _ := HashPassword("correct horse battery staple")
	if other == h {
		t.Fatal("salt not random")
	}
	for _, bad := range []string{"", "$2a$10$abc", "$argon2id$v=19$m=0,t=2,p=1$AAAA$AAAA", "$argon2id$v=18$m=1,t=1,p=1$AAAA$AAAA"} {
		if _, err := VerifyPassword(bad, "x"); err == nil {
			t.Errorf("accepted hash %q", bad)
		}
	}
}

func TestTokens(t *testing.T) {
	a, d, err := NewToken(AccessPrefix)
	if err != nil || !WellFormed(a, AccessPrefix) || WellFormed(a, RefreshPrefix) || string(Digest(a)) != string(d) {
		t.Fatalf("token %s", a)
	}
	b, _, _ := NewToken(AccessPrefix)
	if a == b || WellFormed(a+"x", AccessPrefix) {
		t.Fatal("token uniqueness/length")
	}
}

func TestLimiter(t *testing.T) {
	now := time.Unix(0, 0)
	l := NewLimiter(3, time.Minute)
	l.now = func() time.Time { return now }
	for i := 0; i < 3; i++ {
		if l.Blocked("a") {
			t.Fatal("blocked early")
		}
		l.Fail("a")
	}
	if !l.Blocked("a") || l.Blocked("b") {
		t.Fatal("limit not applied per key")
	}
	now = now.Add(time.Minute)
	if l.Blocked("a") {
		t.Fatal("window did not expire")
	}
	l.Fail("a")
	l.Reset("a")
	if l.Blocked("a") {
		t.Fatal("reset")
	}
}
