package edge_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"log/slog"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	identityv1 "github.com/buidangphuc/team-gateway/generated/platform/identity/v1"
	"github.com/buidangphuc/team-gateway/generated/platform/identity/v1/identityv1connect"
	"github.com/buidangphuc/team-gateway/internal/edge"
	"github.com/buidangphuc/team-gateway/internal/token"
)

type exportSpy struct {
	identityv1.PrivacyServiceClient
	calls  atomic.Int32
	script []error
}

func (s *exportSpy) ExportMyData(context.Context, *identityv1.ExportMyDataRequest, ...grpc.CallOption) (*identityv1.ExportMyDataResponse, error) {
	n := int(s.calls.Add(1))
	if n <= len(s.script) && s.script[n-1] != nil {
		return nil, s.script[n-1]
	}
	return &identityv1.ExportMyDataResponse{Data: []byte(`{"ok":true}`), ContentType: "application/json"}, nil
}

type exportFixture struct {
	client identityv1connect.PrivacyServiceClient
	spy    *exportSpy
	key    *rsa.PrivateKey
}

func newExportFixture(t *testing.T, script ...error) *exportFixture {
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
	spy := &exportSpy{script: script}
	e := edge.NewEdge(token.NewVerifier(jwks.URL, time.Minute), []string{"listing.read"}, time.Second, 3, 1000, 1000)
	opts := connect.WithInterceptors(e.Interceptors(slog.New(slog.DiscardHandler))...)
	mux := http.NewServeMux()
	p, h := identityv1connect.NewPrivacyServiceHandler(edge.NewPrivacyForwarder(spy, e), opts)
	mux.Handle(p, h)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return &exportFixture{client: identityv1connect.NewPrivacyServiceClient(srv.Client(), srv.URL), spy: spy, key: key}
}

func (f *exportFixture) bearer(t *testing.T) string {
	t.Helper()
	claims := &token.Claims{
		Type:   "user",
		Scopes: []string{"order.read"},
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user-1",
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-time.Minute)),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = "kid-1"
	s, err := tok.SignedString(f.key)
	if err != nil {
		t.Fatal(err)
	}
	return "Bearer " + s
}

// Scenario: E5 Anonymous caller is refused
func TestExportMyDataAnonymousRefusedAtEdge(t *testing.T) {
	f := newExportFixture(t)
	_, err := f.client.ExportMyData(context.Background(), privacyReq(&identityv1.ExportMyDataRequest{}, ""))
	if connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("anonymous ExportMyData = %v, want unauthenticated", err)
	}
	if n := f.spy.calls.Load(); n != 0 {
		t.Fatalf("identity calls = %d, want 0", n)
	}
}

// Scenario: E5 Anonymous caller is refused (spoofed principal headers stay anonymous)
func TestExportMyDataSpoofedHeadersRefusedAtEdge(t *testing.T) {
	f := newExportFixture(t)
	r := privacyReq(&identityv1.ExportMyDataRequest{}, "")
	r.Header().Set("X-Principal-Id", "user-1")
	r.Header().Set("X-Principal-Type", "user")
	_, err := f.client.ExportMyData(context.Background(), r)
	if connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("spoofed ExportMyData = %v, want unauthenticated", err)
	}
	if n := f.spy.calls.Load(); n != 0 {
		t.Fatalf("identity calls = %d, want 0", n)
	}
}

// Scenario: E14 The edge response is not cacheable
func TestExportMyDataSetsNoStore(t *testing.T) {
	f := newExportFixture(t)
	res, err := f.client.ExportMyData(context.Background(), privacyReq(&identityv1.ExportMyDataRequest{}, f.bearer(t)))
	if err != nil {
		t.Fatalf("ExportMyData: %v", err)
	}
	if got := res.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
	if string(res.Msg.GetData()) != `{"ok":true}` {
		t.Fatalf("data = %q", res.Msg.GetData())
	}
}

// Scenario: E14 The edge response is not cacheable (read-retry on Unavailable)
func TestExportMyDataRetriesUnavailableAsRead(t *testing.T) {
	f := newExportFixture(t, status.Error(codes.Unavailable, "down"))
	res, err := f.client.ExportMyData(context.Background(), privacyReq(&identityv1.ExportMyDataRequest{}, f.bearer(t)))
	if err != nil {
		t.Fatalf("ExportMyData after one Unavailable: %v", err)
	}
	if n := f.spy.calls.Load(); n != 2 {
		t.Fatalf("identity calls = %d, want 2", n)
	}
	if got := res.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
}

// Scenario: E14 The edge response is not cacheable (upstream error returns no data)
func TestExportMyDataUpstreamErrorPropagates(t *testing.T) {
	f := newExportFixture(t, status.Error(codes.PermissionDenied, "no"))
	_, err := f.client.ExportMyData(context.Background(), privacyReq(&identityv1.ExportMyDataRequest{}, f.bearer(t)))
	if connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("ExportMyData = %v, want permission denied", err)
	}
	if n := f.spy.calls.Load(); n != 1 {
		t.Fatalf("identity calls = %d, want 1 (no retry)", n)
	}
}
