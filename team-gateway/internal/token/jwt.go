// Package token verifies the platform's RS256 JWTs at the edge (ADR-0006).
// team-identity is the issuer; the gateway holds no signing material and instead
// verifies each token against the RSA public key it fetches from identity's JWKS,
// selected by the token's `kid`. A token that does not verify is rejected by the
// edge as Unauthenticated (RFC 6750 §3.1); only the *absence* of a token resolves
// to the anonymous principal.
package token

import (
	"context"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Claims are the platform's JWT claims: a Principal (sub/type/scopes) + name.
type Claims struct {
	Name   string   `json:"name"`
	Type   string   `json:"typ"` // "user" | "service" | "anonymous"
	Scopes []string `json:"scopes"`
	// SessionID is the `sid` claim: the id of the identity session the token was
	// issued for. Empty on service tokens. The edge checks it against the
	// revocation denylist (ADR-0003 addendum).
	SessionID string `json:"sid,omitempty"`
	jwt.RegisteredClaims
}

// Verifier verifies RS256 tokens against a cached JWKS keyset fetched from
// team-identity. It never holds a private key.
type Verifier struct {
	keys *keySet
}

// NewVerifier builds a verifier that fetches and caches the JWKS at jwksURL,
// refreshing on cacheTTL (and on an unknown kid). The first fetch is lazy; call
// Prime to warm the cache at startup without hard-failing boot.
func NewVerifier(jwksURL string, cacheTTL time.Duration) *Verifier {
	if cacheTTL <= 0 {
		cacheTTL = 5 * time.Minute
	}
	return &Verifier{keys: newHTTPKeySet(jwksURL, cacheTTL)}
}

// Prime does a best-effort initial JWKS fetch. Callers should log but not fail
// on error — anonymous read paths still work and the cache fills on first use.
func (v *Verifier) Prime(ctx context.Context) error {
	return v.keys.refresh(ctx)
}

// Verify parses and validates a bearer token, returning its claims. It requires
// RS256, reads the token's kid, and matches it to a public key in the JWKS. A
// token must carry `exp` and a non-empty `sub`.
func (v *Verifier) Verify(tokenString string) (*Claims, error) {
	if tokenString == "" {
		return nil, fmt.Errorf("token: empty token")
	}
	claims := &Claims{}
	_, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("token: unexpected signing method: %v", t.Header["alg"])
		}
		kid, _ := t.Header["kid"].(string)
		if kid == "" {
			return nil, fmt.Errorf("token: missing kid")
		}
		return v.keys.keyForKID(kid)
	}, jwt.WithValidMethods([]string{"RS256"}), jwt.WithExpirationRequired())
	if err != nil {
		return nil, err
	}
	// A token without a subject identifies nobody: reject it (the edge reports any
	// Verify error as an invalid token). iss/aud are not required: identity does
	// not set them.
	if claims.Subject == "" {
		return nil, fmt.Errorf("token: missing sub")
	}
	return claims, nil
}
