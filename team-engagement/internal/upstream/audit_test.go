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

	auditv1 "github.com/buidangphuc/team-engagement/generated/platform/audit/v1"
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

// Scenario: No audit upstream configured.
func TestNewAuditClientEmptyAddrReturnsNil_NoAuditUpstreamConfigured(t *testing.T) {
	c, err := NewAuditClient("", time.Second)
	if err != nil || c != nil {
		t.Fatalf("NewAuditClient(\"\") = %v, %v; want nil, nil", c, err)
	}
	if err := c.Close(); err != nil { // nil-safe Close, like OrderClient
		t.Fatalf("Close on nil client: %v", err)
	}
}

// Scenario: Audit is called as the service principal; also the event carries the
// required fields (Audit event carries the required fields, client half).
func TestAuditClientUsesServicePrincipal(t *testing.T) {
	rec := &recordingAudit{}
	c, err := NewAuditClient(startAudit(t, rec), time.Second)
	if err != nil || c == nil {
		t.Fatalf("NewAuditClient: %v, %v", c, err)
	}
	defer func() { _ = c.Close() }()

	// The admin's own principal rides on the inbound ctx as outgoing metadata;
	// it must be replaced, never forwarded.
	ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs(
		"x-principal-id", "adm-1", "x-principal-type", "user", "x-principal-scopes", "admin"))
	if err := c.WriteHideEvent(ctx, "adm-1", "r-1", "L-1", "spam link"); err != nil {
		t.Fatalf("WriteHideEvent: %v", err)
	}

	if len(rec.reqs) != 1 {
		t.Fatalf("want exactly 1 WriteAuditEvent call, got %d", len(rec.reqs))
	}
	req := rec.reqs[0]
	if req.GetActorId() != "adm-1" || req.GetAction() != "review.hide" ||
		req.GetTargetType() != "review" || req.GetTargetId() != "r-1" {
		t.Errorf("unexpected event: %+v", req)
	}
	if m := req.GetMetadata(); len(m) != 2 || m["reason"] != "spam link" || m["listing_id"] != "L-1" {
		t.Errorf("metadata = %v, want reason+listing_id only", m)
	}
	md := rec.mds[0]
	want := map[string]string{
		"x-principal-id":     "service-team-engagement",
		"x-principal-type":   "service",
		"x-principal-scopes": "audit.write",
	}
	for k, v := range want {
		if vals := md.Get(k); len(vals) != 1 || vals[0] != v {
			t.Errorf("%s = %v, want [%s]", k, vals, v)
		}
	}
}

// Scenario: Audit failure leaves the review visible (client returns the error).
func TestAuditClientUpstreamErrorIsReturned_AuditFailureLeavesReviewVisible(t *testing.T) {
	rec := &recordingAudit{err: status.Error(codes.Internal, "audit db down")}
	c, err := NewAuditClient(startAudit(t, rec), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	if err := c.WriteHideEvent(context.Background(), "adm-1", "r-1", "L-1", "spam"); err == nil {
		t.Fatal("an audit failure must be returned, not swallowed")
	}
}

// Scenario: Audit failure leaves the review visible (timeout half).
func TestAuditClientBoundedWhenAuditHangs_AuditFailureLeavesReviewVisible(t *testing.T) {
	c, err := NewAuditClient(startAudit(t, &recordingAudit{block: true}), 150*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	start := time.Now()
	// No inbound deadline: only the per-call timeout can stop it.
	if err := c.WriteHideEvent(context.Background(), "adm-1", "r-1", "L-1", "spam"); err == nil {
		t.Fatal("expected an error when team-audit hangs")
	}
	if el := time.Since(start); el > 2*time.Second {
		t.Fatalf("audit call was not bounded: took %v", el)
	}
}

// A nil client must fail closed (the service maps it to Unavailable), never succeed silently.
func TestAuditClientNilReceiverFailsClosed_NoAuditUpstreamConfigured(t *testing.T) {
	var c *AuditClient
	if err := c.WriteHideEvent(context.Background(), "adm-1", "r-1", "L-1", "spam"); err == nil {
		t.Fatal("nil audit client must return an error, not nil")
	}
}

// Scenario: Dispute audit: written as the service principal
func TestAuditClientWriteEventUsesServicePrincipal(t *testing.T) {
	rec := &recordingAudit{}
	c, err := NewAuditClient(startAudit(t, rec), time.Second)
	if err != nil || c == nil {
		t.Fatalf("NewAuditClient: %v, %v", c, err)
	}
	defer func() { _ = c.Close() }()

	ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs(
		"x-principal-id", "adm-1", "x-principal-type", "user", "x-principal-scopes", "admin"))
	ev := AuditEvent{
		ActorID: "adm-1", Action: "dispute.resolve", TargetType: "dispute", TargetID: "d-1",
		Metadata: map[string]string{"resolution": "refund buyer", "order_id": "o-1", "status": "resolved"},
	}
	if err := c.WriteEvent(ctx, ev); err != nil {
		t.Fatalf("WriteEvent: %v", err)
	}
	if len(rec.reqs) != 1 {
		t.Fatalf("want exactly 1 call, got %d", len(rec.reqs))
	}
	req := rec.reqs[0]
	if req.GetActorId() != "adm-1" || req.GetAction() != "dispute.resolve" || req.GetTargetType() != "dispute" || req.GetTargetId() != "d-1" {
		t.Errorf("unexpected event: %+v", req)
	}
	if m := req.GetMetadata(); len(m) != 3 || m["resolution"] != "refund buyer" || m["order_id"] != "o-1" || m["status"] != "resolved" {
		t.Errorf("metadata = %v", m)
	}
	want := map[string]string{
		"x-principal-id":     "service-team-engagement",
		"x-principal-type":   "service",
		"x-principal-scopes": "audit.write",
	}
	for k, v := range want {
		if vals := rec.mds[0].Get(k); len(vals) != 1 || vals[0] != v {
			t.Errorf("%s = %v, want [%s]", k, vals, v)
		}
	}
}

// Scenario: Dispute audit: Hide-review audit behaviour is unchanged
func TestWriteHideEventStillBuildsReviewHideEvent(t *testing.T) {
	rec := &recordingAudit{}
	c, err := NewAuditClient(startAudit(t, rec), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	if err := c.WriteHideEvent(context.Background(), "adm-1", "r-1", "L-1", "spam"); err != nil {
		t.Fatal(err)
	}
	if len(rec.reqs) != 1 {
		t.Fatalf("want 1 call, got %d", len(rec.reqs))
	}
	req := rec.reqs[0]
	if req.GetAction() != "review.hide" || req.GetTargetType() != "review" || req.GetTargetId() != "r-1" || req.GetActorId() != "adm-1" {
		t.Errorf("unexpected event: %+v", req)
	}
	if m := req.GetMetadata(); len(m) != 2 || m["reason"] != "spam" || m["listing_id"] != "L-1" {
		t.Errorf("metadata = %v", m)
	}
}

// Scenario: Dispute audit: timeout is Unavailable (client half)
func TestAuditClientWriteEventTimeout(t *testing.T) {
	c, err := NewAuditClient(startAudit(t, &recordingAudit{block: true}), 150*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	start := time.Now()
	err = c.WriteEvent(context.Background(), AuditEvent{ActorID: "adm-1", Action: "dispute.resolve", TargetType: "dispute", TargetID: "d-1"})
	if err == nil {
		t.Fatal("expected an error when team-audit hangs")
	}
	if el := time.Since(start); el > 1150*time.Millisecond {
		t.Fatalf("WriteEvent not bounded by timeout+1s: took %v", el)
	}
}

// Scenario: Dispute audit: No audit upstream configured (nil client fails closed)
func TestAuditClientWriteEventNilReceiverFailsClosed(t *testing.T) {
	var c *AuditClient
	if err := c.WriteEvent(context.Background(), AuditEvent{Action: "dispute.resolve"}); err == nil {
		t.Fatal("nil audit client must return an error")
	}
}

// Scenario: Listing reports: Audit is called as the service principal
// (also: Audit event carries the required fields, client half).
func TestAuditClientUsesServicePrincipalForReports(t *testing.T) {
	rec := &recordingAudit{}
	c, err := NewAuditClient(startAudit(t, rec), time.Second)
	if err != nil || c == nil {
		t.Fatalf("NewAuditClient: %v, %v", c, err)
	}
	defer func() { _ = c.Close() }()

	ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs(
		"x-principal-id", "adm-1", "x-principal-type", "user", "x-principal-scopes", "admin"))
	if err := c.WriteReportResolveEvent(ctx, "adm-1", "rp-1", "L-1", "ACTIONED", "đã gỡ tin"); err != nil {
		t.Fatalf("WriteReportResolveEvent: %v", err)
	}
	if len(rec.reqs) != 1 {
		t.Fatalf("want exactly 1 WriteAuditEvent call, got %d", len(rec.reqs))
	}
	req := rec.reqs[0]
	if req.GetActorId() != "adm-1" || req.GetAction() != "report.resolve" ||
		req.GetTargetType() != "report" || req.GetTargetId() != "rp-1" {
		t.Errorf("unexpected event: %+v", req)
	}
	m := req.GetMetadata()
	if len(m) != 3 || m["status"] != "ACTIONED" || m["listing_id"] != "L-1" || m["resolution_note"] != "đã gỡ tin" {
		t.Errorf("metadata = %v, want exactly status+listing_id+resolution_note", m)
	}
	for k := range m {
		if k == "reporter_id" || k == "note" {
			t.Errorf("metadata must never carry %q", k)
		}
	}
	want := map[string]string{
		"x-principal-id":     "service-team-engagement",
		"x-principal-type":   "service",
		"x-principal-scopes": "audit.write",
	}
	for k, v := range want {
		if vals := rec.mds[0].Get(k); len(vals) != 1 || vals[0] != v {
			t.Errorf("%s = %v, want [%s] (admin principal must be replaced)", k, vals, v)
		}
	}
}

// Scenario: Listing reports: Audit failure leaves the report open (client half: error returned).
func TestAuditClientReportResolveUpstreamErrorReturned_AuditFailureLeavesReportOpen(t *testing.T) {
	rec := &recordingAudit{err: status.Error(codes.Internal, "audit db down")}
	c, err := NewAuditClient(startAudit(t, rec), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	if err := c.WriteReportResolveEvent(context.Background(), "adm-1", "rp-1", "L-1", "DISMISSED", "n"); err == nil {
		t.Fatal("an audit failure must be returned, not swallowed")
	}
}

// Scenario: Listing reports: No audit upstream configured (nil client fails closed).
func TestAuditClientReportResolveNilReceiverFailsClosed_NoAuditUpstreamConfigured(t *testing.T) {
	var c *AuditClient
	if err := c.WriteReportResolveEvent(context.Background(), "adm-1", "rp-1", "L-1", "ACTIONED", "n"); err == nil {
		t.Fatal("nil audit client must return an error")
	}
}
