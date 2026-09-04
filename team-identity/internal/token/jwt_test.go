package token_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/buidangphuc/team-identity/internal/token"
)

// newTestSigner generates a throwaway RSA keypair, PEM-encodes the private key,
// and builds a Signer from it — exercising the real NewSigner PEM parse path.
func newTestSigner(t *testing.T, kid string) (*token.Signer, *rsa.PublicKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
	s, err := token.NewSigner(string(pemBytes), kid)
	if err != nil {
		t.Fatalf("NewSigner: %v", err)
	}
	return s, &key.PublicKey
}

func TestSignRS256(t *testing.T) {
	signer, pub := newTestSigner(t, "kid-1")

	t.Run("signs an RS256 token parseable by the public key with the kid in the header", func(t *testing.T) {
		tok, err := signer.Sign("user-123", "Test User", "user", []string{"listing.read", "listing.write"}, time.Hour)
		if err != nil {
			t.Fatalf("unexpected error signing token: %v", err)
		}
		if tok == "" {
			t.Fatalf("expected non-empty token string")
		}

		// Verify with the PUBLIC key only — the edge never has the private key.
		claims := &token.Claims{}
		parsed, err := jwt.ParseWithClaims(tok, claims, func(tk *jwt.Token) (any, error) {
			if _, ok := tk.Method.(*jwt.SigningMethodRSA); !ok {
				t.Fatalf("expected RSA signing method, got %v", tk.Header["alg"])
			}
			return pub, nil
		})
		if err != nil {
			t.Fatalf("verify with public key: %v", err)
		}
		if parsed.Header["alg"] != "RS256" {
			t.Errorf("expected alg RS256, got %v", parsed.Header["alg"])
		}
		if parsed.Header["kid"] != "kid-1" {
			t.Errorf("expected kid kid-1, got %v", parsed.Header["kid"])
		}
		if claims.Subject != "user-123" {
			t.Errorf("expected subject user-123, got %s", claims.Subject)
		}
		if claims.Name != "Test User" {
			t.Errorf("expected name Test User, got %s", claims.Name)
		}
		if len(claims.Scopes) != 2 {
			t.Errorf("expected 2 scopes, got %d", len(claims.Scopes))
		}
	})

	t.Run("a token signed by a different key does not verify against this public key", func(t *testing.T) {
		other, _ := newTestSigner(t, "kid-2")
		tok, err := other.Sign("user-9", "Other", "user", nil, time.Hour)
		if err != nil {
			t.Fatalf("sign: %v", err)
		}
		_, err = jwt.ParseWithClaims(tok, &token.Claims{}, func(*jwt.Token) (any, error) { return pub, nil })
		if err == nil {
			t.Errorf("expected verification to fail for a token signed by a different key")
		}
	})

	t.Run("expired token is rejected", func(t *testing.T) {
		tok, err := signer.Sign("user-123", "Test User", "user", []string{"listing.read"}, -time.Minute)
		if err != nil {
			t.Fatalf("sign: %v", err)
		}
		_, err = jwt.ParseWithClaims(tok, &token.Claims{}, func(*jwt.Token) (any, error) { return pub, nil })
		if err == nil {
			t.Errorf("expected error for expired token")
		}
	})
}

func TestNewSignerRejectsBadInput(t *testing.T) {
	if _, err := token.NewSigner("not a pem", "kid-1"); err == nil {
		t.Errorf("expected error for invalid PEM")
	}
	_, pub := newTestSigner(t, "kid-1")
	_ = pub
	// missing kid
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	if _, err := token.NewSigner(string(pemBytes), "  "); err == nil {
		t.Errorf("expected error for empty kid")
	}
}

func TestBuildJWKS(t *testing.T) {
	signer, pub := newTestSigner(t, "kid-active")
	set := token.BuildJWKS(token.PublicKey{Kid: signer.KID(), Key: pub})

	if len(set.Keys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(set.Keys))
	}
	k := set.Keys[0]
	if k.Kid != "kid-active" {
		t.Errorf("expected kid kid-active, got %s", k.Kid)
	}
	if k.Kty != "RSA" || k.Use != "sig" || k.Alg != "RS256" {
		t.Errorf("unexpected JWK metadata: %+v", k)
	}
	if k.N == "" || k.E == "" {
		t.Errorf("expected non-empty modulus/exponent, got n=%q e=%q", k.N, k.E)
	}
}
