package token_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/buidangphuc/team-gateway/internal/token"
)

// --- test JWKS server ---------------------------------------------------------

type testJWK struct {
	Kty string `json:"kty"`
	Use string `json:"use"`
	Alg string `json:"alg"`
	Kid string `json:"kid"`
	N   string `json:"n"`
	E   string `json:"e"`
}

// jwksServer serves a mutable JWKS document (mirrors team-identity's publisher)
// so the verifier exercises its real HTTP fetch/parse/cache path.
type jwksServer struct {
	mu   sync.Mutex
	keys map[string]*rsa.PublicKey
	srv  *httptest.Server
}

func newJWKSServer() *jwksServer {
	js := &jwksServer{keys: map[string]*rsa.PublicKey{}}
	js.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		js.mu.Lock()
		defer js.mu.Unlock()
		doc := struct {
			Keys []testJWK `json:"keys"`
		}{}
		for kid, pub := range js.keys {
			doc.Keys = append(doc.Keys, testJWK{
				Kty: "RSA", Use: "sig", Alg: "RS256", Kid: kid,
				N: base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
				E: base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
			})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(doc)
	}))
	return js
}

func (js *jwksServer) publish(kid string, pub *rsa.PublicKey) {
	js.mu.Lock()
	js.keys[kid] = pub
	js.mu.Unlock()
}

// url returns the server's base URL; the handler answers on any path.
func (js *jwksServer) url() string { return js.srv.URL }

func (js *jwksServer) close() { js.srv.Close() }

// signRS256 mints a token with the given key + kid, as team-identity would.
func signRS256(t *testing.T, key *rsa.PrivateKey, kid, subject string) string {
	t.Helper()
	claims := &token.Claims{
		Name:   subject,
		Type:   "user",
		Scopes: []string{"listing.read"},
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   subject,
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = kid
	s, err := tok.SignedString(key)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return s
}

func genKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("genkey: %v", err)
	}
	return k
}

// --- tests --------------------------------------------------------------------

func TestVerifierAcceptsCurrentKey(t *testing.T) {
	js := newJWKSServer()
	defer js.close()
	key := genKey(t)
	js.publish("kid-1", &key.PublicKey)

	v := token.NewVerifier(js.url(), time.Minute)
	if err := v.Prime(context.Background()); err != nil {
		t.Fatalf("prime: %v", err)
	}

	tok := signRS256(t, key, "kid-1", "user-1")
	claims, err := v.Verify(tok)
	if err != nil {
		t.Fatalf("expected valid token to verify, got %v", err)
	}
	if claims.Subject != "user-1" {
		t.Errorf("expected subject user-1, got %s", claims.Subject)
	}
}

func TestVerifierRejectsUnknownKey(t *testing.T) {
	js := newJWKSServer()
	defer js.close()
	known := genKey(t)
	js.publish("kid-1", &known.PublicKey)

	v := token.NewVerifier(js.url(), time.Minute)
	if err := v.Prime(context.Background()); err != nil {
		t.Fatalf("prime: %v", err)
	}

	// A token signed by a key the JWKS never published (its kid is absent).
	rogue := genKey(t)
	tok := signRS256(t, rogue, "kid-rogue", "attacker")
	if _, err := v.Verify(tok); err == nil {
		t.Fatal("expected rejection for a token whose kid is not in the JWKS")
	}

	// A token whose kid matches a published key but is signed by a different key.
	spoof := signRS256(t, rogue, "kid-1", "attacker")
	if _, err := v.Verify(spoof); err == nil {
		t.Fatal("expected rejection for a token whose signature does not match the published key")
	}
}

func TestVerifierAcceptsRotatedInKey(t *testing.T) {
	js := newJWKSServer()
	defer js.close()
	key1 := genKey(t)
	js.publish("kid-1", &key1.PublicKey)

	// Short TTL so the cache is considered stale and refreshes on the new kid.
	v := token.NewVerifier(js.url(), 1*time.Millisecond)
	if err := v.Prime(context.Background()); err != nil {
		t.Fatalf("prime: %v", err)
	}

	// Rotate: publish a second key alongside the first, and sign with the new one.
	key2 := genKey(t)
	js.publish("kid-2", &key2.PublicKey)
	time.Sleep(5 * time.Millisecond) // ensure the cache is now past its TTL

	tok2 := signRS256(t, key2, "kid-2", "user-2")
	if _, err := v.Verify(tok2); err != nil {
		t.Fatalf("expected rotated-in key to verify after refresh, got %v", err)
	}

	// The still-published previous key also continues to verify.
	tok1 := signRS256(t, key1, "kid-1", "user-1")
	if _, err := v.Verify(tok1); err != nil {
		t.Fatalf("expected previous key to keep verifying, got %v", err)
	}
}

func TestVerifierRejectsEmptyToken(t *testing.T) {
	v := token.NewVerifier("http://127.0.0.1:0/.well-known/jwks.json", time.Minute)
	if _, err := v.Verify(""); err == nil {
		t.Error("expected error for empty token")
	}
}
