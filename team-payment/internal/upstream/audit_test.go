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

	auditv1 "github.com/buidangphuc/team-payment/generated/platform/audit/v1"
)

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

// Scenario: Payment audit: no audit upstream configured
func TestNewAuditClientEmptyAddrIsNil(t *testing.T) {
	c, err := NewAuditClient("", time.Second)
	if err != nil || c != nil {
		t.Fatalf("NewAuditClient(\"\") = %v, %v; want nil, nil", c, err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("Close on nil client: %v", err)
	}
	// A nil client must fail closed, never succeed silently.
	if err := c.WriteAdminRefund(context.Background(), "adm-1", "P-1", "O-1", 200000); err == nil {
		t.Fatal("nil audit client must return an error")
	}
}

// Scenario: Audit is called as the service principal (and the event carries the
// action, target, actor and exactly order_id + amount).
func TestAuditClientUsesServicePrincipal(t *testing.T) {
	rec := &recordingAudit{}
	c, err := NewAuditClient(startAudit(t, rec), time.Second)
	if err != nil || c == nil {
		t.Fatalf("NewAuditClient: %v, %v", c, err)
	}
	defer func() { _ = c.Close() }()

	ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs(
		"x-principal-id", "adm-1", "x-principal-type", "user", "x-principal-scopes", "admin"))
	if err := c.WriteAdminRefund(ctx, "adm-1", "P-1", "O-1", 200000); err != nil {
		t.Fatalf("WriteAdminRefund: %v", err)
	}
	if len(rec.reqs) != 1 {
		t.Fatalf("want exactly 1 WriteAuditEvent call, got %d", len(rec.reqs))
	}
	req := rec.reqs[0]
	if req.GetActorId() != "adm-1" || req.GetAction() != "payment.admin_refund" ||
		req.GetTargetType() != "payment" || req.GetTargetId() != "P-1" {
		t.Errorf("unexpected event: %+v", req)
	}
	if m := req.GetMetadata(); len(m) != 2 || m["order_id"] != "O-1" || m["amount"] != "200000" {
		t.Errorf("metadata = %v, want order_id+amount only", m)
	}
	want := map[string]string{
		"x-principal-id":     "service-team-payment",
		"x-principal-type":   "service",
		"x-principal-scopes": "audit.write",
	}
	for k, v := range want {
		if vals := rec.mds[0].Get(k); len(vals) != 1 || vals[0] != v {
			t.Errorf("%s = %v, want [%s]", k, vals, v)
		}
	}
}

// Scenario: Audit error leaves the ledger unchanged (client half: error is returned).
func TestAuditClientUpstreamErrorIsReturned(t *testing.T) {
	rec := &recordingAudit{err: status.Error(codes.Internal, "audit db down")}
	c, err := NewAuditClient(startAudit(t, rec), time.Second)
	if err != nil || c == nil {
		t.Fatalf("NewAuditClient: %v, %v", c, err)
	}
	defer func() { _ = c.Close() }()
	if err := c.WriteAdminRefund(context.Background(), "adm-1", "P-1", "O-1", 1); err == nil {
		t.Fatal("an audit failure must be returned, not swallowed")
	}
	if len(rec.reqs) != 1 {
		t.Fatalf("single attempt expected (no retry), got %d calls", len(rec.reqs))
	}
}

// Scenario: Audit timeout is bounded and refuses the refund (client half)
func TestAuditClientTimeoutBounded(t *testing.T) {
	rec := &recordingAudit{block: true}
	c, err := NewAuditClient(startAudit(t, rec), 150*time.Millisecond)
	if err != nil || c == nil {
		t.Fatalf("NewAuditClient: %v, %v", c, err)
	}
	defer func() { _ = c.Close() }()
	start := time.Now()
	if err := c.WriteAdminRefund(context.Background(), "adm-1", "P-1", "O-1", 1); err == nil {
		t.Fatal("expected an error when team-audit hangs")
	}
	if el := time.Since(start); el > 1150*time.Millisecond {
		t.Fatalf("audit call was not bounded by timeout+1s: took %v", el)
	}
	if len(rec.reqs) != 1 {
		t.Fatalf("no retry expected, got %d calls", len(rec.reqs))
	}
}
