package edge_test

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"log/slog"
	"math/big"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/golang-jwt/jwt/v5"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/kafkax"
	"github.com/buidangphuc/team-gateway/internal/edge"
	"github.com/buidangphuc/team-gateway/internal/token"
	"github.com/buidangphuc/team-gateway/internal/upstream"
)

const fpSecret = "s3cret"

var fpRe = regexp.MustCompile(`^[0-9a-f]{12}$`)

type fpFixture struct {
	srv  *httptest.Server
	logs *syncBuf
	key  *rsa.PrivateKey
}

func newFPFixture(t *testing.T, secret string, rps float64, burst int) *fpFixture {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	jwks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kty": "RSA", "use": "sig", "alg": "RS256", "kid": "kid-1",
			"n": b64(key.N.Bytes()),
			"e": b64(big.NewInt(int64(key.PublicKey.E)).Bytes()),
		}}})
	}))
	t.Cleanup(jwks.Close)
	fp, err := kafkax.NewFingerprinter(secret)
	if err != nil {
		t.Fatal(err)
	}
	logs := &syncBuf{}
	e := edge.NewEdge(token.NewVerifier(jwks.URL, time.Minute), []string{"search:read"}, time.Second, 0, rps, burst).
		WithFingerprinter(fp)
	srv := httptest.NewServer(edge.NewMux(&upstream.Clients{Search: &ctxCapture{}}, e, nil, edge.CockpitConfig{}, slog.New(slog.NewJSONHandler(logs, nil))))
	t.Cleanup(srv.Close)
	return &fpFixture{srv: srv, logs: logs, key: key}
}

func (f *fpFixture) mint(t *testing.T, typ, sub string) string {
	t.Helper()
	claims := &token.Claims{
		Type:   typ,
		Scopes: []string{"search:read"},
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   sub,
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-time.Hour)),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = "kid-1"
	s, err := tok.SignedString(f.key)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func (f *fpFixture) grpc(t *testing.T, bearer string) error {
	t.Helper()
	hdr := map[string]string{}
	if bearer != "" {
		hdr["Authorization"] = "Bearer " + bearer
	}
	return search(t, f.srv, hdr)
}

func (f *fpFixture) http(t *testing.T, bearer string) int {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, f.srv.URL+"/api/session/principal", nil)
	if err != nil {
		t.Fatal(err)
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := f.srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	return resp.StatusCode
}

func (f *fpFixture) lastEdgeLine(t *testing.T, route string) map[string]any {
	t.Helper()
	var raw string
	for _, l := range strings.Split(strings.TrimSpace(f.logs.String()), "\n") {
		if strings.Contains(l, `"msg":"edge.request"`) && strings.Contains(l, route) {
			raw = l
		}
	}
	if raw == "" {
		t.Fatalf("no edge.request line for %q:\n%s", route, f.logs.String())
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func wantFP(t *testing.T, secret, id string) string {
	t.Helper()
	fp, err := kafkax.NewFingerprinter(secret)
	if err != nil {
		t.Fatal(err)
	}
	return fp.Of([]byte(id))
}

func assertKind(t *testing.T, m map[string]any, kind, fp string) {
	t.Helper()
	if _, ok := m["principal"]; ok {
		t.Errorf("edge.request still has a principal key: %v", m)
	}
	if m["principal_kind"] != kind {
		t.Errorf("principal_kind = %v, want %q: %v", m["principal_kind"], kind, m)
	}
	got, has := m["principal_fp"]
	if fp == "" {
		if has {
			t.Errorf("principal_fp must be absent, got %v", got)
		}
		return
	}
	if got != fp {
		t.Errorf("principal_fp = %v, want %q", got, fp)
	}
	if s, _ := got.(string); !fpRe.MatchString(s) {
		t.Errorf("principal_fp %q is not 12 lowercase hex", s)
	}
}

func assertNoRaw(t *testing.T, f *fpFixture, raw string) {
	t.Helper()
	if strings.Contains(f.logs.String(), raw) {
		t.Errorf("log leaks %q:\n%s", raw, f.logs.String())
	}
}

const searchRoute = "SearchListings"

// Scenario: GW-FP-1 gRPC request by a user logs fingerprint and kind
func TestGWFP1_GRPCUserLogsFingerprintAndKind(t *testing.T) {
	f := newFPFixture(t, fpSecret, 1000, 1000)
	if err := f.grpc(t, f.mint(t, "user", "user-1")); err != nil {
		t.Fatal(err)
	}
	assertKind(t, f.lastEdgeLine(t, searchRoute), "user", wantFP(t, fpSecret, "user-1"))
	assertNoRaw(t, f, "user-1")
}

// Scenario: GW-FP-2 HTTP request by a user logs fingerprint and kind
func TestGWFP2_HTTPUserLogsFingerprintAndKind(t *testing.T) {
	f := newFPFixture(t, fpSecret, 1000, 1000)
	f.http(t, f.mint(t, "user", "user-1"))
	assertKind(t, f.lastEdgeLine(t, "/api/session/principal"), "user", wantFP(t, fpSecret, "user-1"))
	assertNoRaw(t, f, "user-1")
}

// Scenario: GW-FP-3 service principal is classified
func TestGWFP3_ServicePrincipalIsClassified(t *testing.T) {
	f := newFPFixture(t, fpSecret, 1000, 1000)
	if err := f.grpc(t, f.mint(t, "service", "svc-order")); err != nil {
		t.Fatal(err)
	}
	assertKind(t, f.lastEdgeLine(t, searchRoute), "service", wantFP(t, fpSecret, "svc-order"))
	assertNoRaw(t, f, "svc-order")
}

// Scenario: GW-FP-4a anonymous gRPC caller has kind and no fingerprint
func TestGWFP4a_AnonymousGRPCHasKindNoFingerprint(t *testing.T) {
	f := newFPFixture(t, fpSecret, 1000, 1000)
	if err := f.grpc(t, ""); err != nil {
		t.Fatal(err)
	}
	assertKind(t, f.lastEdgeLine(t, searchRoute), "anonymous", "")
}

// Scenario: GW-FP-4b anonymous HTTP caller has kind and no fingerprint
func TestGWFP4b_AnonymousHTTPHasKindNoFingerprint(t *testing.T) {
	f := newFPFixture(t, fpSecret, 1000, 1000)
	f.http(t, "")
	assertKind(t, f.lastEdgeLine(t, "/api/session/principal"), "anonymous", "")
}

// Scenario: GW-FP-5 invalid token is not logged as a user
func TestGWFP5_InvalidTokenLoggedAsAnonymous(t *testing.T) {
	f := newFPFixture(t, fpSecret, 1000, 1000)
	err := f.grpc(t, "not.a.token")
	if connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("code = %v, want unauthenticated", connect.CodeOf(err))
	}
	assertKind(t, f.lastEdgeLine(t, searchRoute), "anonymous", "")
}

// Scenario: GW-FP-8 rate-limited gRPC request still fingerprints
func TestGWFP8_RateLimitedGRPCStillFingerprints(t *testing.T) {
	f := newFPFixture(t, fpSecret, 0.0001, 1)
	tok := f.mint(t, "user", "user-1")
	_ = f.grpc(t, tok)
	err := f.grpc(t, tok)
	if connect.CodeOf(err) != connect.CodeResourceExhausted {
		t.Fatalf("code = %v (%v), want resource_exhausted", connect.CodeOf(err), err)
	}
	m := f.lastEdgeLine(t, searchRoute)
	if m["code"] != "resource_exhausted" {
		t.Fatalf("last line code = %v, want resource_exhausted", m["code"])
	}
	assertKind(t, m, "user", wantFP(t, fpSecret, "user-1"))
	assertNoRaw(t, f, "user-1")
}

// Scenario: GW-FP-8 rate-limited HTTP request still fingerprints
func TestGWFP8_RateLimitedHTTPStillFingerprints(t *testing.T) {
	f := newFPFixture(t, fpSecret, 0.0001, 1)
	tok := f.mint(t, "user", "user-1")
	f.http(t, tok)
	if got := f.http(t, tok); got != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", got)
	}
	m := f.lastEdgeLine(t, "/api/session/principal")
	if m["status"] != float64(http.StatusTooManyRequests) {
		t.Fatalf("last line status = %v, want 429", m["status"])
	}
	assertKind(t, m, "user", wantFP(t, fpSecret, "user-1"))
	assertNoRaw(t, f, "user-1")
}

// Scenario: GW-FP-1 fingerprint depends on the configured secret
func TestGWFP1_FingerprintFollowsConfiguredSecret(t *testing.T) {
	a := newFPFixture(t, "secret-a", 1000, 1000)
	b := newFPFixture(t, "secret-b", 1000, 1000)
	if err := a.grpc(t, a.mint(t, "user", "user-1")); err != nil {
		t.Fatal(err)
	}
	if err := b.grpc(t, b.mint(t, "user", "user-1")); err != nil {
		t.Fatal(err)
	}
	ma, mb := a.lastEdgeLine(t, searchRoute), b.lastEdgeLine(t, searchRoute)
	assertKind(t, ma, "user", wantFP(t, "secret-a", "user-1"))
	assertKind(t, mb, "user", wantFP(t, "secret-b", "user-1"))
	if ma["principal_fp"] == mb["principal_fp"] {
		t.Errorf("fingerprints equal across secrets: %v", ma["principal_fp"])
	}
}
