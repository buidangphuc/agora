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

	auditv1 "github.com/buidangphuc/team-promotion/generated/platform/audit/v1"
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

// Scenario: Empty address yields no client.
func TestNewAuditClientEmptyAddrNil(t *testing.T) {
	c, err := NewAuditClient("", time.Second)
	if err != nil || c != nil {
		t.Fatalf("NewAuditClient(\"\") = %v, %v; want nil, nil", c, err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("Close on nil client: %v", err)
	}
}

// Scenario: Promotion audit is called as the service principal.
func TestAuditClientUsesServicePrincipal(t *testing.T) {
	rec := &recordingAudit{}
	c, err := NewAuditClient(startAudit(t, rec), time.Second)
	if err != nil || c == nil {
		t.Fatalf("NewAuditClient: %v, %v", c, err)
	}
	defer func() { _ = c.Close() }()

	// The admin's own principal rides on the context as outgoing metadata and
	// must be replaced, never forwarded.
	ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs(
		"x-principal-id", "adm-1", "x-principal-type", "user", "x-principal-scopes", "admin"))
	err = c.WriteEvent(ctx, AuditEvent{
		ActorID: "adm-1", Action: "voucher.platform_create", TargetType: "voucher", TargetID: "PLAT10",
		Metadata: map[string]string{"discount_type": "DISCOUNT_TYPE_PERCENT", "value": "10"},
	})
	if err != nil {
		t.Fatalf("WriteEvent: %v", err)
	}
	if len(rec.reqs) != 1 {
		t.Fatalf("want exactly 1 WriteAuditEvent call, got %d", len(rec.reqs))
	}
	req := rec.reqs[0]
	if req.GetActorId() != "adm-1" || req.GetAction() != "voucher.platform_create" ||
		req.GetTargetType() != "voucher" || req.GetTargetId() != "PLAT10" {
		t.Errorf("unexpected event: %+v", req)
	}
	if m := req.GetMetadata(); len(m) != 2 || m["discount_type"] != "DISCOUNT_TYPE_PERCENT" || m["value"] != "10" {
		t.Errorf("metadata = %v", m)
	}
	md := rec.mds[0]
	for k, v := range map[string]string{
		"x-principal-id":     "service-team-promotion",
		"x-principal-type":   "service",
		"x-principal-scopes": "audit.write",
	} {
		if vals := md.Get(k); len(vals) != 1 || vals[0] != v {
			t.Errorf("%s = %v, want [%s]", k, vals, v)
		}
	}
}

// Scenario: Promotion audit timeout blocks the voucher (client half: a hung
// team-audit returns an error within the bound, never hangs).
func TestAuditClientTimeoutBounded(t *testing.T) {
	rec := &recordingAudit{block: true}
	c, err := NewAuditClient(startAudit(t, rec), 200*time.Millisecond)
	if err != nil || c == nil {
		t.Fatalf("NewAuditClient: %v, %v", c, err)
	}
	defer func() { _ = c.Close() }()
	start := time.Now()
	err = c.WriteEvent(context.Background(), AuditEvent{ActorID: "a", Action: "x", TargetType: "t", TargetID: "1"})
	if err == nil {
		t.Fatal("a timed-out audit write must return an error")
	}
	if el := time.Since(start); el > 2500*time.Millisecond {
		t.Fatalf("call took %v, want bounded well under 2.5s", el)
	}
}

// Scenario: Promotion audit timeout blocks the voucher (an upstream error is
// returned, not swallowed).
func TestAuditClientUpstreamErrorIsReturned(t *testing.T) {
	rec := &recordingAudit{err: status.Error(codes.Internal, "audit db down")}
	c, err := NewAuditClient(startAudit(t, rec), time.Second)
	if err != nil || c == nil {
		t.Fatalf("NewAuditClient: %v, %v", c, err)
	}
	defer func() { _ = c.Close() }()
	if err := c.WriteEvent(context.Background(), AuditEvent{ActorID: "a", Action: "x", TargetType: "t", TargetID: "1"}); err == nil {
		t.Fatal("an audit failure must be returned, not swallowed")
	}
}
