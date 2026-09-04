// JWKS publication (ADR-0006). team-identity serves its RSA *public* key(s) as a
// standard JWKS document at GET /.well-known/jwks.json so the edge can verify
// RS256 tokens by `kid` without ever holding signing material. The document can
// carry more than one key so a rotated-in key is published before it signs.
package token

import (
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
)

// JWKSPath is the conventional well-known location of the JWKS document.
const JWKSPath = "/.well-known/jwks.json"

// PublicKey pairs an RSA public key with the kid it is published under.
type PublicKey struct {
	Kid string
	Key *rsa.PublicKey
}

// jwk is a single RSA public key in JWK form (RFC 7517 / 7518).
type jwk struct {
	Kty string `json:"kty"`
	Use string `json:"use"`
	Alg string `json:"alg"`
	Kid string `json:"kid"`
	N   string `json:"n"`
	E   string `json:"e"`
}

// JWKS is the published key set: a `keys` array of RSA public JWKs.
type JWKS struct {
	Keys []jwk `json:"keys"`
}

// BuildJWKS renders the given public keys as a JWKS document. Only public key
// material (modulus `n` + exponent `e`) is emitted — never a private key.
func BuildJWKS(keys ...PublicKey) JWKS {
	out := JWKS{Keys: make([]jwk, 0, len(keys))}
	for _, k := range keys {
		if k.Key == nil {
			continue
		}
		out.Keys = append(out.Keys, jwk{
			Kty: "RSA",
			Use: "sig",
			Alg: "RS256",
			Kid: k.Kid,
			N:   base64.RawURLEncoding.EncodeToString(k.Key.N.Bytes()),
			E:   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(k.Key.E)).Bytes()),
		})
	}
	return out
}

// JWKSHandler returns an http.Handler serving the (immutable) JWKS document for
// the given keys at JWKSPath. Public, unauthenticated, read-only — no auth and no
// business logic. The document is marshalled once at construction.
func JWKSHandler(keys ...PublicKey) http.Handler {
	doc, err := json.Marshal(BuildJWKS(keys...))
	if err != nil {
		doc = []byte(`{"keys":[]}`)
	}
	mux := http.NewServeMux()
	mux.HandleFunc(JWKSPath, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "public, max-age=300")
		_, _ = w.Write(doc)
	})
	return mux
}
