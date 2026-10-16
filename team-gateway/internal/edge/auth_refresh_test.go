package edge_test

import (
	"context"
	"io"
	"log/slog"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	identityv1 "github.com/buidangphuc/team-gateway/generated/platform/identity/v1"
	"github.com/buidangphuc/team-gateway/generated/platform/identity/v1/identityv1connect"
	"github.com/buidangphuc/team-gateway/internal/edge"
	"github.com/buidangphuc/team-gateway/internal/token"
	"github.com/buidangphuc/team-gateway/internal/upstream"
)

type fakeRefreshIdentity struct {
	identityv1.AuthServiceClient
	calls atomic.Int32
	err   error
	mu    sync.Mutex
	md    metadata.MD
	req   *identityv1.RefreshSessionRequest
}

func (f *fakeRefreshIdentity) RefreshSession(ctx context.Context, in *identityv1.RefreshSessionRequest, _ ...grpc.CallOption) (*identityv1.RefreshSessionResponse, error) {
	f.calls.Add(1)
	f.mu.Lock()
	f.md, _ = metadata.FromOutgoingContext(ctx)
	f.req = in
	f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	return &identityv1.RefreshSessionResponse{Result: &identityv1.AuthResult{}}, nil
}

func newRefreshClient(t *testing.T, id *fakeRefreshIdentity, retryMax int, rps float64, burst int) identityv1connect.AuthServiceClient {
	t.Helper()
	e := edge.NewEdge(token.NewVerifier("http://127.0.0.1:1/jwks", time.Minute), []string{"listing.read"}, time.Second, retryMax, rps, burst)
	srv := httptest.NewServer(edge.NewMux(&upstream.Clients{Identity: id}, e, nil, edge.CockpitConfig{}, slog.New(slog.NewTextHandler(io.Discard, nil))))
	t.Cleanup(srv.Close)
	return identityv1connect.NewAuthServiceClient(srv.Client(), srv.URL)
}

func refreshReq() *connect.Request[identityv1.RefreshSessionRequest] {
	return connect.NewRequest(&identityv1.RefreshSessionRequest{RefreshToken: "rt-1"})
}

// Scenario: Anonymous caller reaches identity.
func TestRefreshSessionForwardedPublic(t *testing.T) {
	id := &fakeRefreshIdentity{}
	c := newRefreshClient(t, id, 2, 1000, 1000)
	res, err := c.RefreshSession(context.Background(), refreshReq())
	if err != nil {
		t.Fatalf("anonymous RefreshSession: %v", err)
	}
	if res.Msg.GetResult() == nil {
		t.Fatal("identity response not returned")
	}
	if n := id.calls.Load(); n != 1 {
		t.Fatalf("identity calls = %d, want 1", n)
	}
	if id.req.GetRefreshToken() != "rt-1" {
		t.Fatalf("refresh_token forwarded = %q", id.req.GetRefreshToken())
	}
}

// Scenario: Anonymous caller reaches identity (client-supplied principal headers are not trusted).
func TestRefreshSessionForwardedPublicDropsSpoofedPrincipalHeaders(t *testing.T) {
	id := &fakeRefreshIdentity{}
	c := newRefreshClient(t, id, 2, 1000, 1000)
	req := refreshReq()
	req.Header().Set("X-Principal-Id", "admin-1")
	req.Header().Set("X-Principal-Type", "user")
	req.Header().Set("X-Principal-Scopes", "admin")
	if _, err := c.RefreshSession(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if id.calls.Load() != 1 {
		t.Fatalf("identity calls = %d, want 1", id.calls.Load())
	}
	if got := id.md.Get("x-principal-id"); len(got) != 1 || got[0] != "anonymous" {
		t.Fatalf("x-principal-id = %v, want [anonymous]", got)
	}
	for _, v := range id.md.Get("x-principal-scopes") {
		if v == "admin" {
			t.Fatalf("spoofed scope forwarded: %v", id.md.Get("x-principal-scopes"))
		}
	}
	for _, v := range id.md.Get("x-principal-type") {
		if v == "user" {
			t.Fatalf("spoofed principal type forwarded: %v", id.md.Get("x-principal-type"))
		}
	}
}

// Scenario: Upstream Unavailable is not retried.
func TestRefreshSessionNotRetried(t *testing.T) {
	id := &fakeRefreshIdentity{err: status.Error(codes.Unavailable, "identity down")}
	c := newRefreshClient(t, id, 3, 1000, 1000)
	_, err := c.RefreshSession(context.Background(), refreshReq())
	if connect.CodeOf(err) != connect.CodeUnavailable {
		t.Fatalf("got %v, want unavailable", err)
	}
	if n := id.calls.Load(); n != 1 {
		t.Fatalf("identity calls = %d, want exactly 1", n)
	}
}

// Scenario: Unauthenticated passes through.
func TestRefreshSessionUnauthenticatedPassthrough(t *testing.T) {
	id := &fakeRefreshIdentity{err: status.Error(codes.Unauthenticated, "invalid refresh token")}
	c := newRefreshClient(t, id, 2, 1000, 1000)
	_, err := c.RefreshSession(context.Background(), refreshReq())
	if connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("got %v, want unauthenticated", err)
	}
	if n := id.calls.Load(); n != 1 {
		t.Fatalf("identity calls = %d, want 1", n)
	}
}

// Scenario: Anonymous rate-limit bucket applies (burst 40, 41st is resource_exhausted).
// The refill rate is near zero here so the burst boundary is deterministic.
func TestRefreshSessionAnonymousRateLimited(t *testing.T) {
	id := &fakeRefreshIdentity{}
	c := newRefreshClient(t, id, 2, 0.0001, 40)
	for i := 1; i <= 40; i++ {
		if _, err := c.RefreshSession(context.Background(), refreshReq()); err != nil {
			t.Fatalf("call %d: %v, want forwarded", i, err)
		}
	}
	_, err := c.RefreshSession(context.Background(), refreshReq())
	if connect.CodeOf(err) != connect.CodeResourceExhausted {
		t.Fatalf("call 41: %v, want resource_exhausted", err)
	}
	if n := id.calls.Load(); n != 40 {
		t.Fatalf("identity calls = %d, want exactly 40", n)
	}
}
