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

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/jwtx"
)

// Claims are the platform's JWT claims (jwtx.Claims, shared with the verifier at
// the edge) made usable by golang-jwt: Principal fields + display name over the
// standard registered claims. Unchanged from ADR-0003. SessionID (`sid`) ties the
// token to a row in team-identity's sessions table so a revoked session can be
// recognised; it is empty for tokens minted without a session.
type Claims struct{ jwtx.Claims }

// The methods below make Claims a jwt.Claims. Zero times are absent claims.

func numericDate(sec int64) *jwt.NumericDate {
	if sec == 0 {
		return nil
	}
	return jwt.NewNumericDate(time.Unix(sec, 0))
}

// GetExpirationTime implements jwt.Claims.
func (c Claims) GetExpirationTime() (*jwt.NumericDate, error) { return numericDate(c.ExpiresAt), nil }

// GetIssuedAt implements jwt.Claims.
func (c Claims) GetIssuedAt() (*jwt.NumericDate, error) { return numericDate(c.IssuedAt), nil }

// GetNotBefore implements jwt.Claims.
func (c Claims) GetNotBefore() (*jwt.NumericDate, error) { return numericDate(c.NotBefore), nil }

// GetIssuer implements jwt.Claims.
func (c Claims) GetIssuer() (string, error) { return c.Issuer, nil }

// GetSubject implements jwt.Claims.
func (c Claims) GetSubject() (string, error) { return c.Subject, nil }

// GetAudience implements jwt.Claims.
func (c Claims) GetAudience() (jwt.ClaimStrings, error) { return jwt.ClaimStrings(c.Audience), nil }

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
	return s.SignWithSession(subject, name, principalType, scopes, "", ttl)
}

// SignWithSession is Sign plus a `sid` claim carrying the session id.
func (s *Signer) SignWithSession(subject, name, principalType string, scopes []string, sessionID string, ttl time.Duration) (string, error) {
	now := time.Now()
	claims := Claims{jwtx.Claims{
		Name:      name,
		Type:      principalType,
		Scopes:    scopes,
		SessionID: sessionID,
		Subject:   subject,
		IssuedAt:  now.Unix(),
		ExpiresAt: now.Add(ttl).Unix(),
	}}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = s.kid
	signed, err := tok.SignedString(s.key)
	if err != nil {
		return "", fmt.Errorf("token: sign: %w", err)
	}
	return signed, nil
}
