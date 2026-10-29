package edge_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/grpc"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/reqctx"
	searchv1 "github.com/buidangphuc/team-gateway/generated/platform/search/v1"
	"github.com/buidangphuc/team-gateway/generated/platform/search/v1/searchv1connect"
	"github.com/buidangphuc/team-gateway/internal/edge"
	"github.com/buidangphuc/team-gateway/internal/token"
	"github.com/buidangphuc/team-gateway/internal/upstream"
)

type ctxCapture struct {
	searchv1.SearchServiceClient
	rid       string
	principal auth.Principal
	have      bool
	ctxPID    string
}

func (c *ctxCapture) SearchListings(ctx context.Context, _ *searchv1.SearchListingsRequest, _ ...grpc.CallOption) (*searchv1.SearchListingsResponse, error) {
	c.rid = reqctx.RequestID(ctx)
	c.ctxPID = reqctx.PrincipalID(ctx)
	c.principal, c.have = auth.FromContext(ctx)
	return &searchv1.SearchListingsResponse{}, nil
}

func jwksVerifier(t *testing.T) (*rsa.PrivateKey, *token.Verifier) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	jwks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kty": "RSA", "use": "sig", "alg": "RS256", "kid": "kid-1",
			"n": b64(key.PublicKey.N.Bytes()),
			"e": b64(big.NewInt(int64(key.PublicKey.E)).Bytes()),
		}}})
	}))
	t.Cleanup(jwks.Close)
	return key, token.NewVerifier(jwks.URL, time.Minute)
}

func search(t *testing.T, srv *httptest.Server, hdr map[string]string) error {
	t.Helper()
	req := connect.NewRequest(&searchv1.SearchListingsRequest{})
	for k, v := range hdr {
		req.Header().Set(k, v)
	}
	_, err := searchv1connect.NewSearchServiceClient(srv.Client(), srv.URL).SearchListings(context.Background(), req)
	return err
}

// Scenario: The edge.request line keeps its fields.
func TestEdgeRequestLineKeepsFields(t *testing.T) {
	logs := &syncBuf{}
	e := edge.NewEdge(nil, []string{"search:read"}, time.Second, 0, 1000, 1000)
	srv := httptest.NewServer(edge.NewMux(&upstream.Clients{Search: &ctxCapture{}}, e, nil, edge.CockpitConfig{}, slog.New(slog.NewJSONHandler(logs, nil))))
	defer srv.Close()
	if err := search(t, srv, map[string]string{"X-Request-Id": "req-9"}); err != nil {
		t.Fatal(err)
	}
	var raw string
	for _, l := range strings.Split(strings.TrimSpace(logs.String()), "\n") {
		if strings.Contains(l, `"msg":"edge.request"`) {
			raw = l
		}
	}
	if raw == "" {
		t.Fatalf("no edge.request line:\n%s", logs.String())
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"method", "principal", "code", "latency_ms", "request_id", "attempts"} {
		if _, ok := m[k]; !ok {
			t.Errorf("edge.request lacks %q: %s", k, raw)
		}
	}
	if m["request_id"] != "req-9" {
		t.Errorf("request_id = %v, want req-9", m["request_id"])
	}
	if n := strings.Count(raw, `"request_id"`); n != 1 {
		t.Errorf("request_id appears %d times, want once: %s", n, raw)
	}
}

// Scenario: A token never reaches the logs.
func TestBearerTokenIsNeverLogged(t *testing.T) {
	logs := &syncBuf{}
	_, v := jwksVerifier(t)
	e := edge.NewEdge(v, []string{"search:read"}, time.Second, 0, 1000, 1000)
	srv := httptest.NewServer(edge.NewMux(&upstream.Clients{Search: &ctxCapture{}}, e, nil, edge.CockpitConfig{}, slog.New(slog.NewJSONHandler(logs, nil))))
	defer srv.Close()
	const tok = "eyJhbGciOiJSUzI1NiJ9.SECRETPAYLOAD0123.SECRETSIGNATURE0123"
	err := search(t, srv, map[string]string{"Authorization": "Bearer " + tok})
	if connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("code = %v (%v), want unauthenticated", connect.CodeOf(err), err)
	}
	for _, frag := range []string{tok, "SECRETPAYLOAD0123", "SECRETSIGNATURE0123"} {
		if strings.Contains(logs.String(), frag) {
			t.Fatalf("log leaks %q:\n%s", frag, logs.String())
		}
	}
}

// Scenario: An anonymous caller gets the public scopes only (principal and request id reach the SDK context).
func TestAnonymousCallerContextCarriesSDKPrincipalAndRequestID(t *testing.T) {
	up := &ctxCapture{}
	e := edge.NewEdge(nil, []string{"listing.read", "search:read"}, time.Second, 0, 1000, 1000)
	srv := httptest.NewServer(edge.NewMux(&upstream.Clients{Search: up}, e, nil, edge.CockpitConfig{}, slog.New(slog.NewTextHandler(io.Discard, nil))))
	defer srv.Close()
	if err := search(t, srv, map[string]string{"X-Request-Id": "req-ctx-1"}); err != nil {
		t.Fatal(err)
	}
	if up.rid != "req-ctx-1" {
		t.Errorf("reqctx.RequestID = %q, want req-ctx-1", up.rid)
	}
	if !up.have {
		t.Fatal("auth.FromContext has no principal on the upstream call context")
	}
	if !up.principal.Anonymous() || up.principal.ID != "anonymous" {
		t.Errorf("principal = %+v, want anonymous", up.principal)
	}
	if got := strings.Join(up.principal.Scopes, ","); got != "listing.read,search:read" {
		t.Errorf("scopes = %q, want the public scopes only", got)
	}
	if up.ctxPID != "anonymous" {
		t.Errorf("reqctx.PrincipalID = %q, want anonymous", up.ctxPID)
	}
}

// Scenario: An invalid bearer is never downgraded to anonymous (a valid one carries its own principal into the SDK context).
func TestVerifiedCallerContextCarriesSDKPrincipal(t *testing.T) {
	key, v := jwksVerifier(t)
	up := &ctxCapture{}
	e := edge.NewEdge(v, []string{"search:read"}, time.Second, 0, 1000, 1000)
	srv := httptest.NewServer(edge.NewMux(&upstream.Clients{Search: up}, e, nil, edge.CockpitConfig{}, slog.New(slog.NewTextHandler(io.Discard, nil))))
	defer srv.Close()
	tok := mint(t, key, "kid-1", time.Now().Add(time.Hour))
	if err := search(t, srv, map[string]string{"Authorization": "Bearer " + tok}); err != nil {
		t.Fatal(err)
	}
	if !up.have || up.principal.ID != "user-1" || up.principal.Type != auth.TypeUser {
		t.Fatalf("principal = %+v (have=%v), want user-1/user", up.principal, up.have)
	}
	if got := strings.Join(up.principal.Scopes, ","); got != "order.read,search:read" {
		t.Errorf("scopes = %q, want the token's scopes", got)
	}
	if up.ctxPID != "user-1" {
		t.Errorf("reqctx.PrincipalID = %q, want user-1", up.ctxPID)
	}
}

const reflectionPath = "/grpc.reflection.v1.ServerReflection/ServerReflectionInfo"

func reflectionStatus(t *testing.T, e *edge.Edge) int {
	t.Helper()
	srv := httptest.NewServer(edge.NewMux(&upstream.Clients{}, e, nil, edge.CockpitConfig{}, slog.New(slog.NewTextHandler(io.Discard, nil))))
	defer srv.Close()
	res, err := srv.Client().Post(srv.URL+reflectionPath, "application/grpc+proto", strings.NewReader(""))
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	return res.StatusCode
}

// Scenario: Reflection is not served by default.
func TestReflectionNotServedByDefault(t *testing.T) {
	if got := reflectionStatus(t, edge.NewEdge(nil, nil, time.Second, 0, 1000, 1000)); got != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", got)
	}
}

// Scenario: Reflection is served locally when enabled.
func TestReflectionServedWhenEnabledLocally(t *testing.T) {
	if got := reflectionStatus(t, edge.NewEdge(nil, nil, time.Second, 0, 1000, 1000).WithReflection(true)); got == http.StatusNotFound {
		t.Fatalf("status = 404, want reflection to be served")
	}
}
