package upstream

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	auditv1 "github.com/buidangphuc/team-verification/generated/platform/audit/v1"
)

// recordingAudit is an in-process fake team-audit (a true external boundary).
type recordingAudit struct {
	auditv1.UnimplementedAuditServiceServer
	mu    sync.Mutex
	reqs  []*auditv1.WriteAuditEventRequest
	mds   []metadata.MD
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

// Scenario: Empty audit address yields no client.
func TestNewAuditClientEmptyAddrNil(t *testing.T) {
	c, err := NewAuditClient("", time.Second)
	if err != nil || c != nil {
		t.Fatalf("NewAuditClient(\"\") = %v, %v; want nil, nil", c, err)
	}
	if err := c.WriteKycReviewEvent(context.Background(), "adm-1", "kyc_1", "u-1", "approve"); err == nil {
		t.Fatal("a nil client must return an error (fail closed)")
	}
	if err := c.Close(); err != nil {
		t.Fatalf("Close on nil client: %v", err)
	}
}

// Scenario: Audit call uses the service principal.
func TestAuditClientUsesServicePrincipal(t *testing.T) {
	rec := &recordingAudit{}
	c, err := NewAuditClient(startAudit(t, rec), time.Second)
	if err != nil || c == nil {
		t.Fatalf("NewAuditClient: %v, %v", c, err)
	}
	defer func() { _ = c.Close() }()

	// The admin's own principal rides on the outgoing ctx; it must be replaced.
	ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs(
		"x-principal-id", "adm-1", "x-principal-type", "user", "x-principal-scopes", "admin"))
	if err := c.WriteKycReviewEvent(ctx, "adm-1", "kyc_1", "u-1", "approve"); err != nil {
		t.Fatalf("WriteKycReviewEvent: %v", err)
	}

	if len(rec.reqs) != 1 {
		t.Fatalf("want exactly 1 WriteAuditEvent call, got %d", len(rec.reqs))
	}
	req := rec.reqs[0]
	if req.GetActorId() != "adm-1" || req.GetAction() != "kyc.review" ||
		req.GetTargetType() != "kyc_submission" || req.GetTargetId() != "kyc_1" {
		t.Errorf("unexpected event: %+v", req)
	}
	if m := req.GetMetadata(); len(m) != 2 || m["decision"] != "approve" || m["user_id"] != "u-1" {
		t.Errorf("metadata = %v, want decision+user_id only", m)
	}
	want := map[string]string{
		"x-principal-id":     "service-team-verification",
		"x-principal-type":   "service",
		"x-principal-scopes": "audit.write",
	}
	for k, v := range want {
		if vals := rec.mds[0].Get(k); len(vals) != 1 || vals[0] != v {
			t.Errorf("%s = %v, want [%s]", k, vals, v)
		}
	}
}

// Scenario: Audit client call is bounded by the timeout.
func TestAuditClientTimeoutBounded(t *testing.T) {
	c, err := NewAuditClient(startAudit(t, &recordingAudit{block: true}), 100*time.Millisecond)
	if err != nil || c == nil {
		t.Fatalf("NewAuditClient: %v, %v", c, err)
	}
	defer func() { _ = c.Close() }()
	start := time.Now()
	if err := c.WriteKycReviewEvent(context.Background(), "adm-1", "kyc_1", "u-1", "approve"); err == nil {
		t.Fatal("expected an error when team-audit hangs")
	}
	if el := time.Since(start); el > time.Second {
		t.Fatalf("audit call not bounded: took %v", el)
	}
}
