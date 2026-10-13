package jwtx

import (
	"bytes"
	"encoding/json"
)

// Audience is the RFC 7519 aud claim. It marshals as an array and unmarshals
// from either a single string or an array.
type Audience []string

// UnmarshalJSON accepts a JSON string or an array of strings.
func (a *Audience) UnmarshalJSON(b []byte) error {
	if bytes.HasPrefix(bytes.TrimSpace(b), []byte(`"`)) {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*a = Audience{s}
		return nil
	}
	var list []string
	if err := json.Unmarshal(b, &list); err != nil {
		return err
	}
	*a = list
	return nil
}

// Claims are the platform's JWT claims: the RFC 7519 registered claims plus
// the principal fields. Times are Unix seconds.
type Claims struct {
	// Name is the display name.
	Name string `json:"name"`
	// Type is the principal type: "user", "service" or "anonymous".
	Type string `json:"typ"`
	// Scopes are the granted scopes.
	Scopes []string `json:"scopes"`
	// SessionID ties the token to a session; empty for tokens without one.
	SessionID string `json:"sid,omitempty"`

	Issuer    string   `json:"iss,omitempty"`
	Subject   string   `json:"sub,omitempty"`
	Audience  Audience `json:"aud,omitempty"`
	ExpiresAt int64    `json:"exp,omitempty"`
	NotBefore int64    `json:"nbf,omitempty"`
	IssuedAt  int64    `json:"iat,omitempty"`
	ID        string   `json:"jti,omitempty"`
}

// JWK is one RSA public key in JWK form (RFC 7517, RFC 7518).
type JWK struct {
	Kty string `json:"kty"`
	Use string `json:"use"`
	Alg string `json:"alg"`
	Kid string `json:"kid"`
	N   string `json:"n"`
	E   string `json:"e"`
}

// JWKS is a JSON Web Key Set document (RFC 7517).
type JWKS struct {
	Keys []JWK `json:"keys"`
}
