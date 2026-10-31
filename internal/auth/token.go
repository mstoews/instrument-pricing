package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"strings"
)

// Token prefixes make credentials recognizable to secret scanners and let
// the server route a bearer value without a database lookup.
const (
	AccessPrefix  = "ipa_"
	RefreshPrefix = "ipr_"
	APIKeyPrefix  = "ipk_"
)

// NewToken returns a 256-bit random token and the digest to persist.
func NewToken(prefix string) (string, []byte, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", nil, err
	}
	t := prefix + base64.RawURLEncoding.EncodeToString(b)
	return t, Digest(t), nil
}

// Digest is a plain SHA-256: tokens carry 256 bits of entropy, so a slow
// hash adds nothing, and lookups stay a single indexed equality.
func Digest(token string) []byte {
	d := sha256.Sum256([]byte(token))
	return d[:]
}

// WellFormed checks prefix and length before any database work.
func WellFormed(token, prefix string) bool {
	return strings.HasPrefix(token, prefix) && len(token) == len(prefix)+43
}

type ctxKey struct{}

// WithPrincipal and FromContext carry the authenticated caller.
func WithPrincipal[T any](c context.Context, p T) context.Context {
	return context.WithValue(c, ctxKey{}, p)
}

func FromContext[T any](c context.Context) (T, bool) {
	p, ok := c.Value(ctxKey{}).(T)
	return p, ok
}
