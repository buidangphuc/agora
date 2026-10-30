package upstream_test

import (
	"context"
	"net"
	"sync"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auditx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
	auditv1 "github.com/buidangphuc/team-identity/generated/platform/audit/v1"
	"github.com/buidangphuc/team-identity/internal/upstream"
)

type recordingAudit struct {
	auditv1.UnimplementedAuditServiceServer
	mu   sync.Mutex
	reqs []*auditv1.WriteAuditEventRequest
	mds  []metadata.MD
	err  error
}

func (r *recordingAudit) WriteAuditEvent(ctx context.Context, req *auditv1.WriteAuditEventRequest) (*auditv1.WriteAuditEventResponse, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	r.mu.Lock()
	r.reqs = append(r.reqs, req)
	r.mds = append(r.mds, md)
	r.mu.Unlock()
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

func startAudit(t *testing.T, svc *recordingAudit) auditx.Writer {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := grpc.NewServer()
	auditv1.RegisterAuditServiceServer(srv, svc)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	conn, err := auditx.Dial(config.Audit{Addr: lis.Addr().String()}, "service-team-identity", "audit.write")
	if err != nil || conn == nil {
		t.Fatalf("auditx.Dial: %v, %v", conn, err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	w := upstream.NewAuditWriter(conn)
	if w == nil {
		t.Fatal("NewAuditWriter returned nil; it must never be nil")
	}
	return w
}

func adminCtx() context.Context {
	ctx := auth.WithPrincipal(context.Background(), auth.Principal{ID: "adm-1", Type: auth.TypeUser, Scopes: []string{"admin"}})
	return metadata.NewOutgoingContext(ctx, metadata.Pairs(
		"x-principal-id", "adm-1", "x-principal-type", "user", "x-principal-scopes", "admin"))
}

// Scenario: The audit client sends the service principal
func TestAuditUsesServicePrincipal(t *testing.T) {
	rec := &recordingAudit{}
	w := startAudit(t, rec)
	ev := auditx.Event{
		ActorID: "adm-1", Action: "user.set_status", TargetType: "user", TargetID: "u-1",
		Metadata: map[string]string{
			"status": "suspended", "reason_code": "abuse", "note": "repeated spam", "until": "2026-11-01T03:30:00Z",
		},
	}
	if err := w.WriteAuditEvent(adminCtx(), ev); err != nil {
		t.Fatalf("WriteAuditEvent: %v", err)
	}
	if rec.calls() != 1 {
		t.Fatalf("want exactly 1 WriteAuditEvent call, got %d", rec.calls())
	}
	req := rec.reqs[0]
	if req.GetActorId() != "adm-1" || req.GetAction() != "user.set_status" ||
		req.GetTargetType() != "user" || req.GetTargetId() != "u-1" {
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
	want := map[string]string{
		"x-principal-id":     "service-team-identity",
		"x-principal-type":   "service",
		"x-principal-scopes": "audit.write",
	}
	for k, v := range want {
		if vals := rec.mds[0].Get(k); len(vals) != 1 || vals[0] != v {
			t.Errorf("%s = %v, want exactly [%s] (admin principal must be replaced, never forwarded)", k, vals, v)
		}
	}
}

// Scenario: The audit client sends the service principal (optional metadata is passed through unchanged)
func TestAuditWriterMapsEventWithoutOptionalMetadata(t *testing.T) {
	rec := &recordingAudit{}
	w := startAudit(t, rec)
	ev := auditx.Event{
		ActorID: "adm-1", Action: "user.set_status", TargetType: "user", TargetID: "u-2",
		Metadata: map[string]string{"status": "banned", "reason_code": "scam"},
	}
	if err := w.WriteAuditEvent(context.Background(), ev); err != nil {
		t.Fatal(err)
	}
	if rec.calls() != 1 {
		t.Fatalf("want 1 call, got %d", rec.calls())
	}
	m := rec.reqs[0].GetMetadata()
	if len(m) != 2 || m["status"] != "banned" || m["reason_code"] != "scam" {
		t.Errorf("metadata = %v, want status+reason_code only", m)
	}
	if got := rec.mds[0].Get("x-principal-id"); len(got) != 1 || got[0] != "service-team-identity" {
		t.Errorf("x-principal-id = %v, want [service-team-identity] even with no caller principal", got)
	}
}

// Scenario: The audit client sends the service principal (upstream failure is returned, never swallowed)
func TestAuditWriterReturnsUpstreamError(t *testing.T) {
	rec := &recordingAudit{err: status.Error(codes.Internal, "audit db down")}
	w := startAudit(t, rec)
	ev := auditx.Event{ActorID: "adm-1", Action: "user.set_status", TargetType: "user", TargetID: "u-1"}
	err := w.WriteAuditEvent(context.Background(), ev)
	if err == nil {
		t.Fatal("an audit failure must be returned, not swallowed")
	}
	if rec.calls() != 1 {
		t.Fatalf("want exactly 1 call, got %d", rec.calls())
	}
	if status.Code(err) != codes.Internal {
		t.Errorf("code = %v, want Internal passed through for the gate to map", status.Code(err))
	}
}
