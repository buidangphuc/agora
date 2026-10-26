package upstream

import (
	"context"
	"log/slog"
	"net"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auditx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"

	auditv1 "github.com/buidangphuc/team-engagement/generated/platform/audit/v1"
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

func dialWriter(t *testing.T, addr string) auditx.Writer {
	t.Helper()
	conn, err := auditx.Dial(config.Audit{Addr: addr}, "service-team-engagement", "audit.write")
	if err != nil || conn == nil {
		t.Fatalf("auditx.Dial: %v, %v", conn, err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return NewAuditWriter(conn)
}

func hideEvent() auditx.Event {
	return auditx.Event{
		ActorID: "adm-1", Action: "review.hide", TargetType: "review", TargetID: "r-1",
		Metadata: map[string]string{"reason": "spam link", "listing_id": "L-1"},
	}
}

func disputeEvent() auditx.Event {
	return auditx.Event{
		ActorID: "adm-1", Action: "dispute.resolve", TargetType: "dispute", TargetID: "d-1",
		Metadata: map[string]string{"resolution": "refund buyer", "order_id": "o-1", "status": "resolved"},
	}
}

func reportEvent() auditx.Event {
	return auditx.Event{
		ActorID: "adm-1", Action: "report.resolve", TargetType: "report", TargetID: "rp-1",
		Metadata: map[string]string{"status": "ACTIONED", "listing_id": "L-1", "resolution_note": "đã gỡ tin"},
	}
}

func adminCtx() context.Context {
	return metadata.NewOutgoingContext(context.Background(), metadata.Pairs(
		"x-principal-id", "adm-1", "x-principal-type", "user", "x-principal-scopes", "admin"))
}

func assertAuditPrincipal(t *testing.T, md metadata.MD) {
	t.Helper()
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

func assertEvent(t *testing.T, req *auditv1.WriteAuditEventRequest, ev auditx.Event) {
	t.Helper()
	if req.GetActorId() != ev.ActorID || req.GetAction() != ev.Action ||
		req.GetTargetType() != ev.TargetType || req.GetTargetId() != ev.TargetID {
		t.Errorf("unexpected event: %+v", req)
	}
	m := req.GetMetadata()
	if len(m) != len(ev.Metadata) {
		t.Errorf("metadata = %v, want %v", m, ev.Metadata)
	}
	for k, v := range ev.Metadata {
		if m[k] != v {
			t.Errorf("metadata[%s] = %q, want %q", k, m[k], v)
		}
	}
}

func writeOnce(t *testing.T, ev auditx.Event) *recordingAudit {
	t.Helper()
	rec := &recordingAudit{}
	w := dialWriter(t, startAudit(t, rec))
	if err := w.WriteAuditEvent(adminCtx(), ev); err != nil {
		t.Fatalf("WriteAuditEvent: %v", err)
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.reqs) != 1 {
		t.Fatalf("want exactly 1 WriteAuditEvent call, got %d", len(rec.reqs))
	}
	return rec
}

// Scenario: Engagement SDK: Audit call carries the audit.write service principal
func TestAuditCallCarriesServicePrincipal(t *testing.T) {
	rec := writeOnce(t, hideEvent())
	assertAuditPrincipal(t, rec.mds[0])
	assertEvent(t, rec.reqs[0], hideEvent())
	for k, vals := range rec.mds[0] {
		for _, v := range vals {
			if len(k) > 11 && k[:11] == "x-principal" && (v == "adm-1" || v == "admin" || v == "user") {
				t.Errorf("caller principal leaked in %s=%v", k, vals)
			}
		}
	}
}

// Scenario: review-moderation: Audit is called as the service principal
func TestAuditClientUsesServicePrincipal(t *testing.T) {
	rec := writeOnce(t, hideEvent())
	assertAuditPrincipal(t, rec.mds[0])
	assertEvent(t, rec.reqs[0], hideEvent())
}

// Scenario: Dispute audit: written as the service principal
func TestAuditClientWriteEventUsesServicePrincipal(t *testing.T) {
	rec := writeOnce(t, disputeEvent())
	assertAuditPrincipal(t, rec.mds[0])
	assertEvent(t, rec.reqs[0], disputeEvent())
}

// Scenario: Listing reports: Audit is called as the service principal
func TestAuditClientUsesServicePrincipalForReports(t *testing.T) {
	rec := writeOnce(t, reportEvent())
	assertAuditPrincipal(t, rec.mds[0])
	assertEvent(t, rec.reqs[0], reportEvent())
	for k := range rec.reqs[0].GetMetadata() {
		if k == "reporter_id" || k == "note" {
			t.Errorf("metadata must never carry %q", k)
		}
	}
}

// Scenario: Audit failure leaves the review visible (writer returns the upstream error, single attempt)
func TestAuditWriterUpstreamErrorIsReturned(t *testing.T) {
	rec := &recordingAudit{err: status.Error(codes.Internal, "audit db down")}
	w := dialWriter(t, startAudit(t, rec))
	if err := w.WriteAuditEvent(context.Background(), hideEvent()); err == nil {
		t.Fatal("an audit failure must be returned, not swallowed")
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.reqs) != 1 {
		t.Fatalf("single attempt expected, got %d calls", len(rec.reqs))
	}
}

// Scenario: Dispute audit: timeout is Unavailable (writer half, within 2 s plus 1 s)
func TestAuditClientWriteEventTimeout(t *testing.T) {
	w := dialWriter(t, startAudit(t, &recordingAudit{block: true}))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	start := time.Now()
	if err := w.WriteAuditEvent(ctx, disputeEvent()); err == nil {
		t.Fatal("expected an error when team-audit hangs")
	}
	if el := time.Since(start); el > 3*time.Second {
		t.Fatalf("WriteAuditEvent not bounded by 2s plus 1s: took %v", el)
	}
}

// Scenario: Engagement SDK: Audit write is bounded
func TestAuditWriterTimeoutIsBounded(t *testing.T) {
	w := dialWriter(t, startAudit(t, &recordingAudit{block: true}))
	gate := auditx.NewGate(w, slog.New(slog.DiscardHandler))
	applied := false
	start := time.Now()
	err := gate.Do(context.Background(), hideEvent(), func(context.Context) error { applied = true; return nil })
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("code = %v (%v), want Unavailable", status.Code(err), err)
	}
	if applied {
		t.Fatal("action must not run when the audit write times out")
	}
	if el := time.Since(start); el > 3*time.Second {
		t.Fatalf("audit call not bounded: took %v", el)
	}
}
