package edge_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	engagementv1 "github.com/buidangphuc/team-gateway/generated/platform/engagement/v1"
	"github.com/buidangphuc/team-gateway/internal/edge"
	"github.com/buidangphuc/team-gateway/internal/upstream"
)

const (
	listDisputesPath      = "/platform.engagement.v1.EngagementService/ListDisputes"
	listOrderDisputesPath = "/platform.engagement.v1.EngagementService/ListOrderDisputes"
	createDisputePath     = "/platform.engagement.v1.EngagementService/CreateDispute"
	resolveDisputePath    = "/platform.engagement.v1.EngagementService/ResolveDispute"
)

type disputeUpstream struct {
	engagementv1.EngagementServiceClient
	listCalls, listOrderCalls, createCalls, resolveCalls atomic.Int64
	err                                                  error

	mu          sync.Mutex
	lastOrderMD metadata.MD
	lastOrder   *engagementv1.ListOrderDisputesRequest
	lastCreate  *engagementv1.CreateDisputeRequest
	lastResolve *engagementv1.ResolveDisputeRequest
}

func (d *disputeUpstream) ListDisputes(context.Context, *engagementv1.ListDisputesRequest, ...grpc.CallOption) (*engagementv1.ListDisputesResponse, error) {
	d.listCalls.Add(1)
	if d.err != nil {
		return nil, d.err
	}
	return &engagementv1.ListDisputesResponse{}, nil
}

func (d *disputeUpstream) ListOrderDisputes(ctx context.Context, in *engagementv1.ListOrderDisputesRequest, _ ...grpc.CallOption) (*engagementv1.ListOrderDisputesResponse, error) {
	d.listOrderCalls.Add(1)
	md, _ := metadata.FromOutgoingContext(ctx)
	d.mu.Lock()
	d.lastOrderMD, d.lastOrder = md, in
	d.mu.Unlock()
	if d.err != nil {
		return nil, d.err
	}
	return &engagementv1.ListOrderDisputesResponse{Disputes: []*engagementv1.Dispute{{Id: "d-1", OrderId: in.OrderId, ReturnId: "r-1"}}}, nil
}

func (d *disputeUpstream) CreateDispute(_ context.Context, in *engagementv1.CreateDisputeRequest, _ ...grpc.CallOption) (*engagementv1.CreateDisputeResponse, error) {
	d.createCalls.Add(1)
	d.mu.Lock()
	d.lastCreate = in
	d.mu.Unlock()
	return &engagementv1.CreateDisputeResponse{}, nil
}

func (d *disputeUpstream) ResolveDispute(_ context.Context, in *engagementv1.ResolveDisputeRequest, _ ...grpc.CallOption) (*engagementv1.ResolveDisputeResponse, error) {
	d.resolveCalls.Add(1)
	d.mu.Lock()
	d.lastResolve = in
	d.mu.Unlock()
	return &engagementv1.ResolveDisputeResponse{}, nil
}

func callDisputeRPC(t *testing.T, up *disputeUpstream, path, body string, auth func(f *cockpitFixture) string) (int, string) {
	t.Helper()
	f := newCockpitFixture(t)
	srv := httptest.NewServer(edge.NewMux(&upstream.Clients{Engagement: up}, f.edge, nil, edge.CockpitConfig{}, slog.New(slog.DiscardHandler)))
	t.Cleanup(srv.Close)
	req, _ := http.NewRequest(http.MethodPost, srv.URL+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if a := auth(f); a != "" {
		req.Header.Set("Authorization", a)
	}
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(b)
}

// Scenario: Dispute edge: Anonymous ListDisputes is refused.
func TestListDisputesAnonymousUnauthenticated(t *testing.T) {
	up := &disputeUpstream{}
	code, body := callDisputeRPC(t, up, listDisputesPath, `{}`, anonAuth)
	if code != http.StatusUnauthorized || !strings.Contains(body, "unauthenticated") {
		t.Fatalf("anonymous ListDisputes = %d %s, want 401 unauthenticated", code, body)
	}
	if n := up.listCalls.Load(); n != 0 {
		t.Fatalf("upstream calls = %d, want 0", n)
	}
}

// Scenario: Dispute edge: Non-admin ListDisputes is refused.
func TestListDisputesNonAdminPermissionDenied(t *testing.T) {
	up := &disputeUpstream{}
	code, body := callDisputeRPC(t, up, listDisputesPath, `{}`, func(f *cockpitFixture) string {
		return "Bearer " + f.token(t, "engagement:write")
	})
	if code != http.StatusForbidden || !strings.Contains(body, "permission_denied") {
		t.Fatalf("buyer ListDisputes = %d %s, want 403 permission_denied", code, body)
	}
	if n := up.listCalls.Load(); n != 0 {
		t.Fatalf("upstream calls = %d, want 0", n)
	}
}

// Scenario: Dispute edge: Admin ListDisputes is forwarded once.
func TestListDisputesForwardedOnceNoRetry(t *testing.T) {
	up := &disputeUpstream{err: status.Error(codes.Unavailable, "down")}
	code, body := callDisputeRPC(t, up, listDisputesPath, `{}`, func(f *cockpitFixture) string {
		return "Bearer " + f.token(t, "admin")
	})
	if code != http.StatusServiceUnavailable || !strings.Contains(body, "unavailable") {
		t.Fatalf("admin ListDisputes = %d %s, want 503 unavailable", code, body)
	}
	if n := up.listCalls.Load(); n != 1 {
		t.Fatalf("upstream calls = %d, want exactly 1", n)
	}
}

// Scenario: Dispute edge: ListOrderDisputes carries the caller's principal.
func TestListOrderDisputesForwardsPrincipal(t *testing.T) {
	up := &disputeUpstream{}
	code, body := callDisputeRPC(t, up, listOrderDisputesPath, `{"order_id":"o-7"}`, func(f *cockpitFixture) string {
		return "Bearer " + f.token(t, "engagement:write")
	})
	if code != http.StatusOK || !strings.Contains(body, "d-1") || !strings.Contains(body, "r-1") {
		t.Fatalf("buyer ListOrderDisputes = %d %s, want 200 with upstream disputes unchanged", code, body)
	}
	if n := up.listOrderCalls.Load(); n != 1 {
		t.Fatalf("upstream calls = %d, want 1", n)
	}
	up.mu.Lock()
	defer up.mu.Unlock()
	if up.lastOrder.GetOrderId() != "o-7" {
		t.Fatalf("order_id = %q, want o-7", up.lastOrder.GetOrderId())
	}
	if got := up.lastOrderMD.Get("x-principal-id"); len(got) != 1 || got[0] != "user-1" {
		t.Fatalf("x-principal-id = %v, want [user-1]", got)
	}
	if got := up.lastOrderMD.Get("x-principal-type"); len(got) != 1 || got[0] != "user" {
		t.Fatalf("x-principal-type = %v, want [user]", got)
	}
}

// Scenario: Dispute edge: Anonymous ListOrderDisputes is refused.
func TestListOrderDisputesAnonymousUnauthenticated(t *testing.T) {
	up := &disputeUpstream{err: status.Error(codes.Unauthenticated, "sign in")}
	code, body := callDisputeRPC(t, up, listOrderDisputesPath, `{"order_id":"o-7"}`, anonAuth)
	if code != http.StatusUnauthorized || !strings.Contains(body, "unauthenticated") {
		t.Fatalf("anonymous ListOrderDisputes = %d %s, want 401 unauthenticated", code, body)
	}
}

// Scenario: Dispute edge: return_id and ruling pass through.
func TestDisputeFieldsPassThrough(t *testing.T) {
	up := &disputeUpstream{}
	code, body := callDisputeRPC(t, up, createDisputePath, `{"order_id":"o-7","defendant_id":"s-1","reason":"x","return_id":"r-1"}`, func(f *cockpitFixture) string {
		return "Bearer " + f.token(t, "engagement:write")
	})
	if code != http.StatusOK {
		t.Fatalf("CreateDispute = %d %s", code, body)
	}
	code, body = callDisputeRPC(t, up, resolveDisputePath, `{"dispute_id":"d-1","status":"DISPUTE_STATUS_RESOLVED","resolution":"ok","ruling":"DISPUTE_RULING_BUYER_FAVOUR"}`, func(f *cockpitFixture) string {
		return "Bearer " + f.token(t, "admin")
	})
	if code != http.StatusOK {
		t.Fatalf("ResolveDispute = %d %s", code, body)
	}
	up.mu.Lock()
	defer up.mu.Unlock()
	if up.lastCreate.GetReturnId() != "r-1" {
		t.Fatalf("return_id = %q, want r-1", up.lastCreate.GetReturnId())
	}
	if up.lastResolve.GetRuling() != engagementv1.DisputeRuling_DISPUTE_RULING_BUYER_FAVOUR {
		t.Fatalf("ruling = %v, want BUYER_FAVOUR", up.lastResolve.GetRuling())
	}
}
