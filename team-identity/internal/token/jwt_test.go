package token_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/jwtx"
	"github.com/buidangphuc/team-identity/internal/token"
)

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

func verify(t *testing.T, tok string, pub *rsa.PublicKey) *jwt.Token {
	t.Helper()
	parsed, err := jwt.Parse(tok, func(tk *jwt.Token) (any, error) {
		if _, ok := tk.Method.(*jwt.SigningMethodRSA); !ok {
			t.Fatalf("expected RSA signing method, got %v", tk.Header["alg"])
		}
		return pub, nil
	}, jwt.WithoutClaimsValidation())
	if err != nil {
		t.Fatalf("verify with public key: %v", err)
	}
	return parsed
}

func payload(t *testing.T, tok string) []byte {
	t.Helper()
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		t.Fatalf("expected 3 token segments, got %d", len(parts))
	}
	b, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	return b
}

func TestSignRS256(t *testing.T) {
	signer, pub := newTestSigner(t, "kid-1")

	t.Run("A signed token decodes as the shared claims", func(t *testing.T) {
		ttl := 45 * time.Minute
		tok, err := signer.SignWithSession("user-123", "Test User", "user", []string{"listing.read", "listing.write"}, "sess-77", ttl)
		if err != nil {
			t.Fatalf("unexpected error signing token: %v", err)
		}
		parsed := verify(t, tok, pub)
		if parsed.Header["alg"] != "RS256" {
			t.Errorf("expected alg RS256, got %v", parsed.Header["alg"])
		}
		if parsed.Header["kid"] != "kid-1" {
			t.Errorf("expected kid kid-1, got %v", parsed.Header["kid"])
		}

		var claims jwtx.Claims
		if err := json.Unmarshal(payload(t, tok), &claims); err != nil {
			t.Fatalf("decode as jwtx.Claims: %v", err)
		}
		if claims.Subject != "user-123" || claims.Name != "Test User" || claims.Type != "user" {
			t.Errorf("unexpected identity claims: %+v", claims)
		}
		if claims.SessionID != "sess-77" {
			t.Errorf("expected sid sess-77, got %q", claims.SessionID)
		}
		if len(claims.Scopes) != 2 || claims.Scopes[0] != "listing.read" || claims.Scopes[1] != "listing.write" {
			t.Errorf("unexpected scopes: %v", claims.Scopes)
		}
		if claims.IssuedAt == 0 || claims.ExpiresAt-claims.IssuedAt != int64(ttl.Seconds()) {
			t.Errorf("expected exp-iat == %d, got iat=%d exp=%d", int64(ttl.Seconds()), claims.IssuedAt, claims.ExpiresAt)
		}
	})

	t.Run("A signed token decodes as the shared claims for Sign without a session", func(t *testing.T) {
		tok, err := signer.Sign("svc-1", "Svc", "service", []string{"audit.write"}, time.Hour)
		if err != nil {
			t.Fatalf("sign: %v", err)
		}
		var claims jwtx.Claims
		if err := json.Unmarshal(payload(t, tok), &claims); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if claims.Subject != "svc-1" || claims.Type != "service" || claims.SessionID != "" {
			t.Errorf("unexpected claims: %+v", claims)
		}
	})

	t.Run("a token signed by a different key does not verify against this public key", func(t *testing.T) {
		other, _ := newTestSigner(t, "kid-2")
		tok, err := other.Sign("user-9", "Other", "user", nil, time.Hour)
		if err != nil {
			t.Fatalf("sign: %v", err)
		}
		_, err = jwt.Parse(tok, func(*jwt.Token) (any, error) { return pub, nil })
		if err == nil {
			t.Errorf("expected verification to fail for a token signed by a different key")
		}
	})

	t.Run("expired token is rejected", func(t *testing.T) {
		tok, err := signer.Sign("user-123", "Test User", "user", []string{"listing.read"}, -time.Minute)
		if err != nil {
			t.Fatalf("sign: %v", err)
		}
		_, err = jwt.Parse(tok, func(*jwt.Token) (any, error) { return pub, nil })
		if err == nil {
			t.Errorf("expected error for expired token")
		}
	})
}

func TestSignedTokenOmitsEmptySID(t *testing.T) {
	signer, _ := newTestSigner(t, "kid-1")

	t.Run("A service token carries no sid", func(t *testing.T) {
		for name, sign := range map[string]func() (string, error){
			"Sign": func() (string, error) {
				return signer.Sign("svc-1", "Svc", "service", []string{"audit.write"}, time.Hour)
			},
			"SignWithSession empty": func() (string, error) {
				return signer.SignWithSession("svc-1", "Svc", "service", []string{"audit.write"}, "", time.Hour)
			},
		} {
			tok, err := sign()
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			var m map[string]any
			if err := json.Unmarshal(payload(t, tok), &m); err != nil {
				t.Fatalf("%s: decode: %v", name, err)
			}
			if _, ok := m["sid"]; ok {
				t.Errorf("%s: payload must have no sid key, got %v", name, m)
			}
			for _, k := range []string{"sub", "name", "typ", "scopes", "iat", "exp"} {
				if _, ok := m[k]; !ok {
					t.Errorf("%s: payload missing %q: %v", name, k, m)
				}
			}
		}
	})

	t.Run("A signed token with a session keeps the sid key", func(t *testing.T) {
		tok, err := signer.SignWithSession("u", "U", "user", nil, "sess-1", time.Hour)
		if err != nil {
			t.Fatalf("sign: %v", err)
		}
		var m map[string]any
		_ = json.Unmarshal(payload(t, tok), &m)
		if m["sid"] != "sess-1" {
			t.Errorf("expected sid sess-1, got %v", m["sid"])
		}
	})
}

func TestNewSignerRejectsBadInput(t *testing.T) {
	if _, err := token.NewSigner("not a pem", "kid-1"); err == nil {
		t.Errorf("expected error for invalid PEM")
	}
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	if _, err := token.NewSigner(string(pemBytes), "  "); err == nil {
		t.Errorf("expected error for empty kid")
	}
}

func goldenKeys() (token.PublicKey, token.PublicKey) {
	a := token.PublicKey{Kid: "kid-a", Key: &rsa.PublicKey{N: new(big.Int).SetBytes([]byte{0xc0, 0xff, 0xee, 0x01, 0x02, 0x03}), E: 65537}}
	b := token.PublicKey{Kid: "kid-b", Key: &rsa.PublicKey{N: new(big.Int).SetBytes([]byte{0x0b, 0xad, 0xf0, 0x0d}), E: 65537}}
	return a, b
}

const (
	goldenKeyA = `{"kty":"RSA","use":"sig","alg":"RS256","kid":"kid-a","n":"wP_uAQID","e":"AQAB"}`
	goldenKeyB = `{"kty":"RSA","use":"sig","alg":"RS256","kid":"kid-b","n":"C63wDQ","e":"AQAB"}`
)

func TestBuildJWKS(t *testing.T) {
	t.Run("The JWKS document is public keys only: shared type and metadata", func(t *testing.T) {
		signer, pub := newTestSigner(t, "kid-active")
		var set jwtx.JWKS = token.BuildJWKS(token.PublicKey{Kid: signer.KID(), Key: pub})
		if len(set.Keys) != 1 {
			t.Fatalf("expected 1 key, got %d", len(set.Keys))
		}
		k := set.Keys[0]
		if k.Kid != "kid-active" || k.Kty != "RSA" || k.Use != "sig" || k.Alg != "RS256" {
			t.Errorf("unexpected JWK metadata: %+v", k)
		}
		if k.N == "" || k.E == "" {
			t.Errorf("expected non-empty modulus/exponent, got n=%q e=%q", k.N, k.E)
		}
	})

	t.Run("The JWKS document is public keys only: golden bytes", func(t *testing.T) {
		a, b := goldenKeys()
		var set jwtx.JWKS = token.BuildJWKS(a, b)
		got, err := json.Marshal(set)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		want := `{"keys":[` + goldenKeyA + `,` + goldenKeyB + `]}`
		if string(got) != want {
			t.Errorf("golden mismatch\n got: %s\nwant: %s", got, want)
		}
	})

	t.Run("a nil key is skipped and an empty set marshals as an empty array", func(t *testing.T) {
		var set jwtx.JWKS = token.BuildJWKS(token.PublicKey{Kid: "x"})
		got, _ := json.Marshal(set)
		if string(got) != `{"keys":[]}` {
			t.Errorf("got %s", got)
		}
	})
}

func TestJWKSHandlerMethodsAndPaths(t *testing.T) {
	a, b := goldenKeys()
	h := token.JWKSHandler(a, b)
	want := `{"keys":[` + goldenKeyA + `,` + goldenKeyB + `]}`

	do := func(method, path string) *http.Response {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
		return rec.Result()
	}

	t.Run("The JWKS document is public keys only: GET serves the golden body", func(t *testing.T) {
		resp := do(http.MethodGet, "/.well-known/jwks.json")
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status %d", resp.StatusCode)
		}
		if string(body) != want {
			t.Errorf("body mismatch\n got: %s\nwant: %s", body, want)
		}
		if cc := resp.Header.Get("Cache-Control"); cc != "public, max-age=300" {
			t.Errorf("Cache-Control %q", cc)
		}
		var set jwtx.JWKS
		if err := json.Unmarshal(body, &set); err != nil || len(set.Keys) != 2 {
			t.Errorf("decode as jwtx.JWKS: %v %+v", err, set)
		}
		var raw struct {
			Keys []map[string]any `json:"keys"`
		}
		_ = json.Unmarshal(body, &raw)
		for _, k := range raw.Keys {
			if len(k) != 6 {
				t.Errorf("key has %d fields, want only kty,use,alg,kid,n,e: %v", len(k), k)
			}
		}
	})

	t.Run("The JWKS listener refuses other methods: HEAD is allowed", func(t *testing.T) {
		resp := do(http.MethodHead, "/.well-known/jwks.json")
		if resp.StatusCode != http.StatusOK {
			t.Errorf("status %d", resp.StatusCode)
		}
		if cc := resp.Header.Get("Cache-Control"); cc != "public, max-age=300" {
			t.Errorf("Cache-Control %q", cc)
		}
	})

	t.Run("The JWKS listener refuses other methods: POST PUT DELETE get 405 with Allow", func(t *testing.T) {
		for _, m := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch} {
			resp := do(m, "/.well-known/jwks.json")
			if resp.StatusCode != http.StatusMethodNotAllowed {
				t.Errorf("%s: status %d", m, resp.StatusCode)
			}
			if al := resp.Header.Get("Allow"); al != "GET, HEAD" {
				t.Errorf("%s: Allow %q", m, al)
			}
		}
	})

	t.Run("The JWKS listener refuses other methods: other paths get 404", func(t *testing.T) {
		for _, p := range []string{"/other", "/", "/.well-known/jwks.json/x"} {
			if resp := do(http.MethodGet, p); resp.StatusCode != http.StatusNotFound {
				t.Errorf("%s: status %d", p, resp.StatusCode)
			}
		}
	})
}
