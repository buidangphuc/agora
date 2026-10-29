package token_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/jwtx"

	"github.com/buidangphuc/team-gateway/internal/token"
)

func countingJWKS(t *testing.T, _ string, hits *atomic.Int64, doc jwtx.JWKS) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(doc)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func sdkDoc(t *testing.T, kid string) (jwtx.JWKS, func() string) {
	t.Helper()
	key := genKey(t)
	pub := key.PublicKey
	doc := jwtx.JWKS{Keys: []jwtx.JWK{{
		Kty: "RSA", Use: "sig", Alg: "RS256", Kid: kid,
		N: base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
		E: base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
	}}}
	return doc, func() string { return signRS256(t, key, kid, "user-sdk") }
}

// Scenario: JWKS keys decode through the SDK types.
func TestJWKSDocumentDecodesWithSDKTypes(t *testing.T) {
	var hits atomic.Int64
	doc, mint := sdkDoc(t, "kid-sdk")
	srv := countingJWKS(t, "kid-sdk", &hits, doc)

	v := token.NewVerifier(srv.URL, time.Minute)
	if err := v.Prime(context.Background()); err != nil {
		t.Fatalf("prime against a jwtx.JWKS document: %v", err)
	}
	claims, err := v.Verify(mint())
	if err != nil {
		t.Fatalf("token signed by a key published as jwtx.JWK must verify: %v", err)
	}
	if claims.Subject != "user-sdk" {
		t.Errorf("subject = %q, want user-sdk", claims.Subject)
	}
}

// Scenario: A current key verifies (readiness).
func TestVerifierReadyAfterFirstFetch(t *testing.T) {
	var hits atomic.Int64
	doc, _ := sdkDoc(t, "kid-sdk")
	srv := countingJWKS(t, "kid-sdk", &hits, doc)

	v := token.NewVerifier(srv.URL, time.Minute)
	if err := v.Check(context.Background()); err == nil {
		t.Fatal("Check must error before any JWKS fetch succeeded")
	}
	if err := v.Prime(context.Background()); err != nil {
		t.Fatalf("prime: %v", err)
	}
	if err := v.Check(context.Background()); err != nil {
		t.Fatalf("Check after a successful fetch: %v", err)
	}
	srv.Close()
	if err := v.Check(context.Background()); err != nil {
		t.Fatalf("Check must stay nil after the JWKS server stops: %v", err)
	}
}

// Scenario: A current key verifies (Check reads state only).
func TestVerifierCheckNeverFetches(t *testing.T) {
	var hits atomic.Int64
	doc, _ := sdkDoc(t, "kid-sdk")
	srv := countingJWKS(t, "kid-sdk", &hits, doc)

	v := token.NewVerifier(srv.URL, time.Nanosecond)
	for range 3 {
		_ = v.Check(context.Background())
	}
	if got := hits.Load(); got != 0 {
		t.Fatalf("Check before prime fetched %d times, want 0", got)
	}
	if err := v.Prime(context.Background()); err != nil {
		t.Fatalf("prime: %v", err)
	}
	primed := hits.Load()
	for range 3 {
		if err := v.Check(context.Background()); err != nil {
			t.Fatalf("Check: %v", err)
		}
	}
	if got := hits.Load(); got != primed {
		t.Fatalf("Check fetched even with a stale cache: hits %d -> %d", primed, got)
	}
}
