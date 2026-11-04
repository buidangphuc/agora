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
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	identityv1 "github.com/buidangphuc/team-gateway/generated/platform/identity/v1"
	"github.com/buidangphuc/team-gateway/generated/platform/identity/v1/identityv1connect"
	"github.com/buidangphuc/team-gateway/internal/edge"
	"github.com/buidangphuc/team-gateway/internal/token"
)

type becomeSellerSpy struct {
	identityv1.AuthServiceClient
	calls atomic.Int32
	err   error
	mu    sync.Mutex
	md    metadata.MD
	req   *identityv1.BecomeSellerRequest
}

func (s *becomeSellerSpy) BecomeSeller(ctx context.Context, in *identityv1.BecomeSellerRequest, _ ...grpc.CallOption) (*identityv1.BecomeSellerResponse, error) {
	s.calls.Add(1)
	s.mu.Lock()
	s.md, _ = metadata.FromOutgoingContext(ctx)
	s.req = in
	s.mu.Unlock()
	if s.err != nil {
		return nil, s.err
	}
	return &identityv1.BecomeSellerResponse{Result: &identityv1.AuthResult{}}, nil
}

type becomeSellerFixture struct {
	client identityv1connect.AuthServiceClient
	spy    *becomeSellerSpy
	key    *rsa.PrivateKey
}

func newBecomeSellerFixture(t *testing.T, upstreamErr error) *becomeSellerFixture {
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

	spy := &becomeSellerSpy{err: upstreamErr}
	e := edge.NewEdge(token.NewVerifier(jwks.URL, time.Minute), []string{"listing.read"}, time.Second, 3, 1000, 1000)
	opts := connect.WithInterceptors(e.Interceptors(slog.New(slog.DiscardHandler))...)
	mux := http.NewServeMux()
	p, h := identityv1connect.NewAuthServiceHandler(edge.NewAuthForwarder(spy, e), opts)
	mux.Handle(p, h)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return &becomeSellerFixture{
		client: identityv1connect.NewAuthServiceClient(srv.Client(), srv.URL),
		spy:    spy,
		key:    key,
	}
}

func (f *becomeSellerFixture) bearer(t *testing.T, ptype, sub, sid string, scopes []string) string {
	t.Helper()
	claims := &token.Claims{
		Type:      ptype,
		Scopes:    scopes,
		SessionID: sid,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   sub,
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

func (f *becomeSellerFixture) call(auth string, extra map[string]string) error {
	req := connect.NewRequest(&identityv1.BecomeSellerRequest{TermsVersion: "v1"})
	if auth != "" {
		req.Header().Set("Authorization", auth)
	}
	for k, v := range extra {
		req.Header().Set(k, v)
	}
	_, err := f.client.BecomeSeller(context.Background(), req)
	return err
}

// Scenario: Seller onboarding: a service or anonymous caller is refused (anonymous).
func TestBecomeSellerRequiresUserPrincipalAnonymous(t *testing.T) {
	f := newBecomeSellerFixture(t, nil)
	err := f.call("", nil)
	if connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("anonymous BecomeSeller = %v, want unauthenticated", err)
	}
	if n := f.spy.calls.Load(); n != 0 {
		t.Fatalf("identity calls = %d, want 0", n)
	}
}

// Scenario: Seller onboarding: a service or anonymous caller is refused (spoofed principal headers stay anonymous).
func TestBecomeSellerRequiresUserPrincipalSpoofedHeaders(t *testing.T) {
	f := newBecomeSellerFixture(t, nil)
	err := f.call("", map[string]string{
		"X-Principal-Id":   "user-1",
		"X-Principal-Type": "user",
		"X-Principal-Sid":  "sid-forged",
	})
	if connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("spoofed-header BecomeSeller = %v, want unauthenticated", err)
	}
	if n := f.spy.calls.Load(); n != 0 {
		t.Fatalf("identity calls = %d, want 0", n)
	}
}

// Scenario: Seller onboarding: a service or anonymous caller is refused (service principal).
func TestBecomeSellerRequiresUserPrincipalService(t *testing.T) {
	f := newBecomeSellerFixture(t, nil)
	err := f.call(f.bearer(t, "service", "svc-order", "", []string{"order.read"}), nil)
	if connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("service BecomeSeller = %v, want permission_denied", err)
	}
	if n := f.spy.calls.Load(); n != 0 {
		t.Fatalf("identity calls = %d, want 0", n)
	}
}

// Scenario: Seller onboarding: a service or anonymous caller is refused (service principal with a sid claim).
func TestBecomeSellerRequiresUserPrincipalServiceWithSid(t *testing.T) {
	f := newBecomeSellerFixture(t, nil)
	err := f.call(f.bearer(t, "service", "svc-order", "sid-1", []string{"admin"}), nil)
	if connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("service BecomeSeller = %v, want permission_denied", err)
	}
	if n := f.spy.calls.Load(); n != 0 {
		t.Fatalf("identity calls = %d, want 0", n)
	}
}

// Scenario: Seller onboarding: a service or anonymous caller is refused (a malformed token is unauthenticated, not downgraded).
func TestBecomeSellerRequiresUserPrincipalInvalidToken(t *testing.T) {
	f := newBecomeSellerFixture(t, nil)
	err := f.call("Bearer not-a-jwt", nil)
	if connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("invalid-token BecomeSeller = %v, want unauthenticated", err)
	}
	if n := f.spy.calls.Load(); n != 0 {
		t.Fatalf("identity calls = %d, want 0", n)
	}
}

// Scenario: Seller onboarding: signed-in buyer through the gateway (a buyer without admin or seller scope is forwarded with sid).
func TestBecomeSellerUserForwardedWithSid(t *testing.T) {
	f := newBecomeSellerFixture(t, nil)
	err := f.call(f.bearer(t, "user", "user-1", "sid-verified", []string{"order.read"}), map[string]string{"X-Principal-Sid": "sid-forged"})
	if err != nil {
		t.Fatalf("user BecomeSeller: %v", err)
	}
	if n := f.spy.calls.Load(); n != 1 {
		t.Fatalf("identity calls = %d, want 1", n)
	}
	if got := f.spy.req.GetTermsVersion(); got != "v1" {
		t.Fatalf("terms_version forwarded = %q, want v1", got)
	}
	if got := f.spy.md.Get("x-principal-sid"); len(got) != 1 || got[0] != "sid-verified" {
		t.Fatalf("x-principal-sid = %v, want [sid-verified]", got)
	}
	if got := f.spy.md.Get("x-principal-id"); len(got) != 1 || got[0] != "user-1" {
		t.Fatalf("x-principal-id = %v, want [user-1]", got)
	}
	if got := f.spy.md.Get("x-principal-type"); len(got) != 1 || got[0] != "user" {
		t.Fatalf("x-principal-type = %v, want [user]", got)
	}
}

// Scenario: Seller onboarding: signed-in buyer through the gateway (the upstream result is returned).
func TestBecomeSellerUserGetsUpstreamResult(t *testing.T) {
	f := newBecomeSellerFixture(t, nil)
	req := connect.NewRequest(&identityv1.BecomeSellerRequest{TermsVersion: "v1"})
	req.Header().Set("Authorization", f.bearer(t, "user", "user-1", "sid-1", []string{"order.read"}))
	res, err := f.client.BecomeSeller(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if res.Msg.GetResult() == nil {
		t.Fatal("identity result not returned")
	}
}

// Scenario: Seller onboarding: signed-in buyer through the gateway (a write is never retried).
func TestBecomeSellerNotRetried(t *testing.T) {
	f := newBecomeSellerFixture(t, status.Error(codes.Unavailable, "identity down"))
	err := f.call(f.bearer(t, "user", "user-1", "sid-1", []string{"order.read"}), nil)
	if connect.CodeOf(err) != connect.CodeUnavailable {
		t.Fatalf("got %v, want unavailable", err)
	}
	if n := f.spy.calls.Load(); n != 1 {
		t.Fatalf("identity calls = %d, want exactly 1", n)
	}
}

// Scenario: Seller onboarding: a service or anonymous caller is refused.
func TestBecomeSellerRequiresUserPrincipal(t *testing.T) {
	cases := []struct {
		name string
		auth func(*testing.T, *becomeSellerFixture) string
		want connect.Code
	}{
		{"anonymous", func(*testing.T, *becomeSellerFixture) string { return "" }, connect.CodeUnauthenticated},
		{"service", func(t *testing.T, f *becomeSellerFixture) string {
			return f.bearer(t, "service", "svc-order", "", []string{"order.read"})
		}, connect.CodePermissionDenied},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newBecomeSellerFixture(t, nil)
			err := f.call(tc.auth(t, f), nil)
			if connect.CodeOf(err) != tc.want {
				t.Fatalf("BecomeSeller = %v, want %v", err, tc.want)
			}
			if n := f.spy.calls.Load(); n != 0 {
				t.Fatalf("identity calls = %d, want 0", n)
			}
		})
	}
}
