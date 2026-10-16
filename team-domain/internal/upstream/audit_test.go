package upstream

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	auditv1 "github.com/buidangphuc/team-domain/generated/platform/audit/v1"
)

// recordingAudit captures what team-audit would receive.
type recordingAudit struct {
	auditv1.UnimplementedAuditServiceServer
	mu    sync.Mutex
	reqs  []*auditv1.WriteAuditEventRequest
	mds   []metadata.MD
	err   error
	block bool
}

func (r *recordingAudit) WriteAuditEvent(ctx context.Context, req *auditv1.WriteAuditEventRequest) (*auditv1.WriteAuditEventResponse, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	r.mu.Lock()
	r.reqs = append(r.reqs, req)
	r.mds = append(r.mds, md)
	r.mu.Unlock()
	if r.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if r.err != nil {
		return nil, r.err
	}
	return &auditv1.WriteAuditEventResponse{}, nil
}

func (r *recordingAudit) calls() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.reqs)
}

func startAudit(t *testing.T, svc *recordingAudit) string {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := grpc.NewServer()
	auditv1.RegisterAuditServiceServer(srv, svc)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	return lis.Addr().String()
}

// Scenario: Listing audit: No audit upstream configured.
func TestNewAuditClientEmptyAddrIsNil(t *testing.T) {
	c, err := NewAuditClient("", time.Second)
	if err != nil || c != nil {
		t.Fatalf("NewAuditClient(\"\") = %v, %v; want nil, nil", c, err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("Close on nil client: %v", err)
	}
	// Use of the nil client must fail closed.
	if err := c.WriteListingAdminUpdate(context.Background(), "adm-1", "L-1", "s-1", "rejected"); err == nil {
		t.Fatal("nil client update must return an error")
	}
	if err := c.WriteListingAdminDelete(context.Background(), "adm-1", "L-1", "s-1"); err == nil {
		t.Fatal("nil client delete must return an error")
	}
}

// Scenario: Listing audit: Audit is called as the service principal.
func TestAuditClientUsesServicePrincipal(t *testing.T) {
	rec := &recordingAudit{}
	c, err := NewAuditClient(startAudit(t, rec), time.Second)
	if err != nil || c == nil {
		t.Fatalf("NewAuditClient: %v, %v", c, err)
	}
	defer func() { _ = c.Close() }()

	ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs(
		"x-principal-id", "adm-1", "x-principal-type", "user", "x-principal-scopes", "admin"))
	if err := c.WriteListingAdminUpdate(ctx, "adm-1", "L-1", "s-1", "rejected"); err != nil {
		t.Fatalf("WriteListingAdminUpdate: %v", err)
	}
	if err := c.WriteListingAdminDelete(ctx, "adm-1", "L-1", "s-1"); err != nil {
		t.Fatalf("WriteListingAdminDelete: %v", err)
	}
	if rec.calls() != 2 {
		t.Fatalf("want 2 WriteAuditEvent calls, got %d", rec.calls())
	}

	up := rec.reqs[0]
	if up.GetActorId() != "adm-1" || up.GetAction() != "listing.admin_update" ||
		up.GetTargetType() != "listing" || up.GetTargetId() != "L-1" {
		t.Errorf("unexpected update event: %+v", up)
	}
	if m := up.GetMetadata(); len(m) != 2 || m["seller_id"] != "s-1" || m["status"] != "rejected" {
		t.Errorf("update metadata = %v, want seller_id+status only", m)
	}
	del := rec.reqs[1]
	if del.GetActorId() != "adm-1" || del.GetAction() != "listing.admin_delete" ||
		del.GetTargetType() != "listing" || del.GetTargetId() != "L-1" {
		t.Errorf("unexpected delete event: %+v", del)
	}
	if m := del.GetMetadata(); len(m) != 1 || m["seller_id"] != "s-1" {
		t.Errorf("delete metadata = %v, want seller_id only", m)
	}

	want := map[string]string{
		"x-principal-id":     "service-team-domain",
		"x-principal-type":   "service",
		"x-principal-scopes": "audit.write",
	}
	for i, md := range rec.mds {
		for k, v := range want {
			if vals := md.Get(k); len(vals) != 1 || vals[0] != v {
				t.Errorf("call %d %s = %v, want [%s]", i, k, vals, v)
			}
		}
	}
}

// Scenario: Listing audit: Slow audit is cut off at the timeout.
func TestAuditClientTimeoutSingleAttempt(t *testing.T) {
	cases := []struct {
		name string
		rec  *recordingAudit
		max  time.Duration
	}{
		{"error response", &recordingAudit{err: status.Error(codes.Internal, "audit db down")}, 2 * time.Second},
		{"deadline exceeded", &recordingAudit{block: true}, 150*time.Millisecond + 500*time.Millisecond},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, err := NewAuditClient(startAudit(t, tc.rec), 150*time.Millisecond)
			if err != nil || c == nil {
				t.Fatalf("NewAuditClient: %v, %v", c, err)
			}
			defer func() { _ = c.Close() }()
			start := time.Now()
			if err := c.WriteListingAdminUpdate(context.Background(), "adm-1", "L-1", "s-1", "draft"); err == nil {
				t.Fatal("expected an error")
			}
			if el := time.Since(start); el > tc.max {
				t.Fatalf("call took %v, want <= %v", el, tc.max)
			}
			if n := tc.rec.calls(); n != 1 {
				t.Fatalf("want exactly 1 attempt (no retry), got %d", n)
			}
		})
	}
}

// Scenario: Listing audit: Caller principal is never forwarded.
func TestAuditClientNeverForwardsCallerPrincipal(t *testing.T) {
	rec := &recordingAudit{}
	c, err := NewAuditClient(startAudit(t, rec), time.Second)
	if err != nil || c == nil {
		t.Fatalf("NewAuditClient: %v, %v", c, err)
	}
	defer func() { _ = c.Close() }()

	ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs(
		"x-principal-id", "seller-9", "x-principal-type", "user", "x-principal-scopes", "listing.write,listing.read"))
	ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(
		"x-principal-id", "seller-9", "x-principal-scopes", "listing.write,listing.read"))
	_ = c.WriteListingAdminDelete(ctx, "adm-1", "L-1", "s-1")

	if rec.calls() != 1 {
		t.Fatalf("want 1 call, got %d", rec.calls())
	}
	for k, vals := range rec.mds[0] {
		for _, v := range vals {
			if v == "seller-9" || v == "listing.write,listing.read" || v == "user" {
				t.Errorf("caller principal leaked in %s=%v", k, vals)
			}
		}
	}
}
