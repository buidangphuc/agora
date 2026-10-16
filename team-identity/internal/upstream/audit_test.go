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

	auditv1 "github.com/buidangphuc/team-identity/generated/platform/audit/v1"
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

// Scenario: User enforcement: No audit upstream configured
func TestNewAuditClientEmptyAddrIsNil(t *testing.T) {
	c, err := NewAuditClient("", time.Second)
	if err != nil || c != nil {
		t.Fatalf("NewAuditClient(\"\") = %v, %v; want nil, nil", c, err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("Close on nil client must be safe: %v", err)
	}
}

// Scenario: User enforcement: No audit upstream configured (nil client fails closed).
func TestAuditNilReceiverFailsClosed_NoAuditUpstreamConfigured(t *testing.T) {
	var c *AuditClient
	if err := c.WriteUserSetStatus(context.Background(), "adm-1", "u-1", "banned", "scam", "", nil); err == nil {
		t.Fatal("nil audit client must return an error, never succeed silently")
	}
}

// Scenario: User enforcement: Audit is called as the service principal
func TestAuditUsesServicePrincipal(t *testing.T) {
	rec := &recordingAudit{}
	c, err := NewAuditClient(startAudit(t, rec), time.Second)
	if err != nil || c == nil {
		t.Fatalf("NewAuditClient: %v, %v", c, err)
	}
	defer func() { _ = c.Close() }()

	// The admin's own principal rides on the context; it must be replaced, never forwarded.
	ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs(
		"x-principal-id", "adm-1", "x-principal-type", "user", "x-principal-scopes", "admin"))
	until := time.Date(2026, 11, 1, 10, 30, 0, 0, time.FixedZone("x", 7*3600))
	if err := c.WriteUserSetStatus(ctx, "adm-1", "u-1", "suspended", "abuse", "repeated spam", &until); err != nil {
		t.Fatalf("WriteUserSetStatus: %v", err)
	}
	if len(rec.reqs) != 1 {
		t.Fatalf("want exactly 1 WriteAuditEvent call, got %d", len(rec.reqs))
	}
	req := rec.reqs[0]
	if req.GetActorId() != "adm-1" || req.GetAction() != "user.set_status" ||
		req.GetTargetType() != "user" || req.GetTargetId() != "u-1" {
		t.Errorf("unexpected event: %+v", req)
	}
	m := req.GetMetadata()
	if len(m) != 4 || m["status"] != "suspended" || m["reason_code"] != "abuse" ||
		m["note"] != "repeated spam" || m["until"] != "2026-11-01T03:30:00Z" {
		t.Errorf("metadata = %v, want status, reason_code, note, until (RFC3339 UTC)", m)
	}
	want := map[string]string{
		"x-principal-id":     "service-team-identity",
		"x-principal-type":   "service",
		"x-principal-scopes": "audit.write",
	}
	for k, v := range want {
		if vals := rec.mds[0].Get(k); len(vals) != 1 || vals[0] != v {
			t.Errorf("%s = %v, want [%s]", k, vals, v)
		}
	}
}

// Scenario: User enforcement: Ban writes one audit event and revokes sessions
// (client half: optional note and until are omitted from metadata when not given).
func TestAuditOmitsNoteAndUntilWhenAbsent(t *testing.T) {
	rec := &recordingAudit{}
	c, err := NewAuditClient(startAudit(t, rec), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	if err := c.WriteUserSetStatus(context.Background(), "adm-1", "u-1", "banned", "scam", "", nil); err != nil {
		t.Fatal(err)
	}
	m := rec.reqs[0].GetMetadata()
	if len(m) != 2 || m["status"] != "banned" || m["reason_code"] != "scam" {
		t.Errorf("metadata = %v, want status+reason_code only", m)
	}
}

// Scenario: User enforcement: Audit down refuses the change (client returns the error).
func TestAuditUpstreamErrorIsReturned(t *testing.T) {
	rec := &recordingAudit{err: status.Error(codes.Internal, "audit db down")}
	c, err := NewAuditClient(startAudit(t, rec), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	if err := c.WriteUserSetStatus(context.Background(), "adm-1", "u-1", "banned", "scam", "", nil); err == nil {
		t.Fatal("an audit failure must be returned, not swallowed")
	}
}

// Scenario: User enforcement: Audit down refuses the change (deadline half: bounded call).
func TestAuditBoundedWhenAuditHangs(t *testing.T) {
	c, err := NewAuditClient(startAudit(t, &recordingAudit{block: true}), 150*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	start := time.Now()
	if err := c.WriteUserSetStatus(context.Background(), "adm-1", "u-1", "banned", "scam", "", nil); err == nil {
		t.Fatal("expected an error when team-audit hangs")
	}
	if el := time.Since(start); el > 2*time.Second {
		t.Fatalf("audit call was not bounded: took %v", el)
	}
}
