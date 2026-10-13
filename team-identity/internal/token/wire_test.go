package token_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/jwtx"

	"github.com/buidangphuc/team-identity/internal/token"
)

// The edge (team-gateway) verifies identity's tokens and reads the claims as
// jwtx.Claims. A token identity issues must decode into it with no unknown
// field, and its payload must carry exactly the claim names the gateway expects
// with exp and iat as numeric seconds.
func TestIssuedTokenDecodesIntoJWTXClaims(t *testing.T) {
	signer, _ := newTestSigner(t, "kid-wire")
	before := time.Now().Unix()
	tok, err := signer.SignWithSession("user-1", "Alice", "user", []string{"order.read", "order.write"}, "sess-9", time.Hour)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		t.Fatalf("want 3 token parts, got %d", len(parts))
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("decode payload: %v", err)
	}

	dec := json.NewDecoder(bytes.NewReader(payload))
	dec.DisallowUnknownFields()
	var c jwtx.Claims
	if err := dec.Decode(&c); err != nil {
		t.Fatalf("payload %s does not decode into jwtx.Claims: %v", payload, err)
	}
	if c.Name != "Alice" || c.Type != "user" || c.SessionID != "sess-9" || c.Subject != "user-1" {
		t.Errorf("claims = %+v", c)
	}
	if len(c.Scopes) != 2 || c.Scopes[0] != "order.read" || c.Scopes[1] != "order.write" {
		t.Errorf("scopes = %v", c.Scopes)
	}
	if c.IssuedAt < before || c.IssuedAt > time.Now().Unix() {
		t.Errorf("iat = %d, want Unix seconds around now", c.IssuedAt)
	}
	if c.ExpiresAt != c.IssuedAt+3600 {
		t.Errorf("exp = %d, want iat+3600 = %d", c.ExpiresAt, c.IssuedAt+3600)
	}

	// Wire shape: these claim names, in this order, exp and iat as JSON integers.
	want := fmt.Sprintf(`{"name":"Alice","typ":"user","scopes":["order.read","order.write"],"sid":"sess-9","sub":"user-1","exp":%d,"iat":%d}`, c.ExpiresAt, c.IssuedAt)
	if string(payload) != want {
		t.Errorf("payload\n got %s\nwant %s", payload, want)
	}
}

// A token without a session omits sid, as before.
func TestTokenWithoutSessionOmitsSid(t *testing.T) {
	signer, _ := newTestSigner(t, "kid-wire")
	tok, err := signer.Sign("svc-1", "Billing", "service", nil, time.Hour)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	payload, err := base64.RawURLEncoding.DecodeString(strings.Split(tok, ".")[1])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), `"sid"`) {
		t.Errorf("payload carries sid: %s", payload)
	}
	if !strings.Contains(string(payload), `"scopes":null`) {
		t.Errorf("payload %s: nil scopes must stay `null` as before", payload)
	}
}

// The JWKS document is byte-for-byte the JSON the gateway has always fetched,
// and it decodes into jwtx.JWKS.
func TestJWKSEndpointJSONShape(t *testing.T) {
	signer, pub := newTestSigner(t, "kid-wire")
	srv := httptest.NewServer(token.JWKSHandler(token.PublicKey{Kid: signer.KID(), Key: pub}))
	defer srv.Close()

	resp, err := http.Get(srv.URL + token.JWKSPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var body bytes.Buffer
	if _, err := body.ReadFrom(resp.Body); err != nil {
		t.Fatal(err)
	}

	n := base64.RawURLEncoding.EncodeToString(pub.N.Bytes())
	e := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes())
	want := fmt.Sprintf(`{"keys":[{"kty":"RSA","use":"sig","alg":"RS256","kid":"kid-wire","n":"%s","e":"%s"}]}`, n, e)
	if body.String() != want {
		t.Errorf("jwks\n got %s\nwant %s", body.String(), want)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("content type = %q", ct)
	}

	var set jwtx.JWKS
	dec := json.NewDecoder(&body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&set); err != nil {
		t.Fatalf("decode into jwtx.JWKS: %v", err)
	}
	if len(set.Keys) != 1 || set.Keys[0].Kid != "kid-wire" {
		t.Errorf("keys = %+v", set.Keys)
	}
}
