// Package token signs the platform's JWTs (ADR-0006). team-identity is the sole
// issuer: it signs RS256 with an RSA private key it alone holds and stamps a
// `kid` in the header so the edge (team-gateway) can pick the matching public
// key from the JWKS this package also publishes. There is no HMAC/HS256 path and
// no Verify here — the issuer never verifies its own tokens; the edge does, using
// only the public key.
package token

import (
	"crypto/rsa"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Claims are the platform's JWT claims: a Principal (sub/type/scopes) + display
// name, over the standard registered claims (exp/iat). Unchanged from ADR-0003.
type Claims struct {
	Name   string   `json:"name"`
	Type   string   `json:"typ"` // "user" | "service" | "anonymous"
	Scopes []string `json:"scopes"`
	jwt.RegisteredClaims
}

// Signer holds the parsed RSA private key and the key id it stamps into every
// token header. It is the only thing that can mint a platform token.
type Signer struct {
	key *rsa.PrivateKey
	kid string
}

// NewSigner parses a PEM-encoded RSA private key (PKCS#1 or PKCS#8) and binds it
// to a stable, non-empty key id.
func NewSigner(privateKeyPEM, kid string) (*Signer, error) {
	if strings.TrimSpace(kid) == "" {
		return nil, fmt.Errorf("token: kid is required")
	}
	normalizedKey := strings.TrimSpace(privateKeyPEM)
	normalizedKey = strings.Trim(normalizedKey, "'\"")
	normalizedKey = strings.ReplaceAll(normalizedKey, `\n`, "\n")
	normalizedKey = strings.ReplaceAll(normalizedKey, `\r`, "\r")
	normalizedKey = strings.TrimSpace(normalizedKey)
	key, err := jwt.ParseRSAPrivateKeyFromPEM([]byte(normalizedKey))
	if err != nil {
		return nil, fmt.Errorf("token: parse RSA private key: %w", err)
	}
	return &Signer{key: key, kid: kid}, nil
}

// KID returns the signer's key id.
func (s *Signer) KID() string { return s.kid }

// PublicKey returns the RSA public key matching this signer, for JWKS publication.
func (s *Signer) PublicKey() *rsa.PublicKey { return &s.key.PublicKey }

// Sign issues an RS256-signed token for a subject with the given scopes, stamping
// the signer's `kid` in the JWT header.
func (s *Signer) Sign(subject, name, principalType string, scopes []string, ttl time.Duration) (string, error) {
	now := time.Now()
	claims := Claims{
		Name:   name,
		Type:   principalType,
		Scopes: scopes,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   subject,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = s.kid
	signed, err := tok.SignedString(s.key)
	if err != nil {
		return "", fmt.Errorf("token: sign: %w", err)
	}
	return signed, nil
}
