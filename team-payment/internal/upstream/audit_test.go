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

func dialWriter(t *testing.T, addr string) auditx.Writer {
	t.Helper()
	conn, err := auditx.Dial(config.Audit{Addr: addr}, "service-team-payment", "audit.write")
	if err != nil || conn == nil {
		t.Fatalf("auditx.Dial: %v, %v", conn, err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return NewAuditWriter(conn)
}

func refundEvent(refundID string) auditx.Event {
	return auditx.Event{
		ActorID: "adm-1", Action: "payment.admin_refund", TargetType: "payment", TargetID: "P-1",
		Metadata: map[string]string{"order_id": "O-1", "refund_id": refundID, "amount": "200000"},
	}
}

// Scenario: The audit call is made as the service principal
func TestAuditWriterUsesServicePrincipal(t *testing.T) {
	rec := &recordingAudit{}
	w := dialWriter(t, startAudit(t, rec))

	ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs(
		"x-principal-id", "adm-1", "x-principal-type", "user", "x-principal-scopes", "admin"))
	if err := w.WriteAuditEvent(ctx, refundEvent("rf-1")); err != nil {
		t.Fatalf("WriteAuditEvent: %v", err)
	}

	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.reqs) != 1 {
		t.Fatalf("want exactly 1 WriteAuditEvent call, got %d", len(rec.reqs))
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

// Scenario: An admin refund writes one event with the SDK event shape
func TestAuditWriterMapsEventToRequest(t *testing.T) {
	rec := &recordingAudit{}
	w := dialWriter(t, startAudit(t, rec))
	if err := w.WriteAuditEvent(context.Background(), refundEvent("rf-1")); err != nil {
		t.Fatalf("WriteAuditEvent: %v", err)
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.reqs) != 1 {
		t.Fatalf("want exactly 1 event, got %d", len(rec.reqs))
	}
	req := rec.reqs[0]
	if req.GetActorId() != "adm-1" || req.GetAction() != "payment.admin_refund" ||
		req.GetTargetType() != "payment" || req.GetTargetId() != "P-1" {
		t.Errorf("unexpected event: %+v", req)
	}
	m := req.GetMetadata()
	if len(m) != 3 || m["order_id"] != "O-1" || m["refund_id"] != "rf-1" || m["amount"] != "200000" {
		t.Errorf("metadata = %v, want exactly order_id+refund_id+amount", m)
	}
}

// Scenario: An admin refund writes one event with the SDK event shape (distinct and repeated refund ids stay verbatim)
func TestAuditWriterRefundIDVerbatim(t *testing.T) {
	rec := &recordingAudit{}
	w := dialWriter(t, startAudit(t, rec))
	ids := []string{"rf-1", "rf-1", "rf-2", "a0123456789012345678901234567890123456789012345678901234567890z"}
	for _, id := range ids {
		if err := w.WriteAuditEvent(context.Background(), refundEvent(id)); err != nil {
			t.Fatalf("WriteAuditEvent(%q): %v", id, err)
		}
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.reqs) != len(ids) {
		t.Fatalf("want %d events, got %d", len(ids), len(rec.reqs))
	}
	for i, id := range ids {
		if got := rec.reqs[i].GetMetadata()["refund_id"]; got != id {
			t.Errorf("event %d refund_id = %q, want %q", i, got, id)
		}
	}
}

// Scenario: An admin refund writes one event with the SDK event shape (an upstream failure is returned, single attempt)
func TestAuditWriterUpstreamErrorIsReturned(t *testing.T) {
	rec := &recordingAudit{err: status.Error(codes.Internal, "audit db down")}
	w := dialWriter(t, startAudit(t, rec))
	if err := w.WriteAuditEvent(context.Background(), refundEvent("rf-1")); err == nil {
		t.Fatal("an audit failure must be returned, not swallowed")
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.reqs) != 1 {
		t.Fatalf("single attempt expected, got %d calls", len(rec.reqs))
	}
}

// Scenario: The audit call is made as the service principal (the gate bounds a hanging audit and the action does not run)
func TestAuditWriterHangIsBoundedByGate(t *testing.T) {
	w := dialWriter(t, startAudit(t, &recordingAudit{block: true}))
	gate := auditx.NewGate(w, slog.New(slog.DiscardHandler))
	applied := false
	start := time.Now()
	err := gate.Do(context.Background(), refundEvent("rf-1"), func(context.Context) error { applied = true; return nil })
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("code = %v (%v), want Unavailable", status.Code(err), err)
	}
	if applied {
		t.Fatal("action must not run when the audit write times out")
	}
	if el := time.Since(start); el > 4*time.Second {
		t.Fatalf("audit call not bounded: took %v", el)
	}
}
