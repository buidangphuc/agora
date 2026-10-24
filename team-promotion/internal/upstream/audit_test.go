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
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"

	auditv1 "github.com/buidangphuc/team-promotion/generated/platform/audit/v1"
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
	conn, err := auditx.Dial(config.Audit{Addr: addr}, "service-team-promotion", "audit.write")
	if err != nil || conn == nil {
		t.Fatalf("auditx.Dial: %v, %v", conn, err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return NewAuditWriter(conn)
}

func voucherEvent() auditx.Event {
	return auditx.Event{
		ActorID: "adm-1", Action: "voucher.platform_create", TargetType: "voucher", TargetID: "PLAT10",
		Metadata: map[string]string{"discount_type": "DISCOUNT_TYPE_PERCENT", "value": "10"},
	}
}

func adminCtx(base context.Context) context.Context {
	return auth.WithPrincipal(base, auth.Principal{ID: "adm-1", Type: auth.TypeUser, Scopes: []string{"admin"}})
}

// Scenario: Audit is written as the service principal
func TestAuditClientUsesServicePrincipal(t *testing.T) {
	base := t.Context()
	rec := &recordingAudit{}
	w := dialWriter(t, startAudit(t, rec))
	ev := voucherEvent()
	if err := w.WriteAuditEvent(adminCtx(base), ev); err != nil {
		t.Fatalf("WriteAuditEvent: %v", err)
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.reqs) != 1 {
		t.Fatalf("want exactly 1 WriteAuditEvent call, got %d", len(rec.reqs))
	}
	req := rec.reqs[0]
	if req.GetActorId() != ev.ActorID || req.GetAction() != ev.Action ||
		req.GetTargetType() != ev.TargetType || req.GetTargetId() != ev.TargetID {
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
	for k, vals := range md {
		for _, v := range vals {
			if len(k) >= 11 && k[:11] == "x-principal" && (v == "adm-1" || v == "admin" || v == "user") {
				t.Errorf("admin principal leaked in %s=%v", k, vals)
			}
		}
	}
}

// Scenario: A slow audit write is bounded
func TestAuditClientTimeoutBounded(t *testing.T) {
	base := t.Context()
	w := dialWriter(t, startAudit(t, &recordingAudit{block: true}))
	gate := auditx.NewGate(w, slog.New(slog.DiscardHandler))
	applied := false
	start := time.Now()
	err := gate.Do(adminCtx(base), voucherEvent(), func(context.Context) error { applied = true; return nil })
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("code = %v (%v), want Unavailable", status.Code(err), err)
	}
	if status.Convert(err).Message() != "audit trail unavailable" {
		t.Errorf("message = %q", status.Convert(err).Message())
	}
	if applied {
		t.Fatal("action must not run when the audit write times out")
	}
	if el := time.Since(start); el > 2500*time.Millisecond {
		t.Fatalf("audit call took %v, want within 2.5s", el)
	}
}

// Scenario: A slow audit write is bounded (an upstream error is returned, single attempt)
func TestAuditClientUpstreamErrorIsReturned(t *testing.T) {
	base := t.Context()
	rec := &recordingAudit{err: status.Error(codes.Internal, "audit db down")}
	w := dialWriter(t, startAudit(t, rec))
	if err := w.WriteAuditEvent(base, voucherEvent()); err == nil {
		t.Fatal("an audit failure must be returned, not swallowed")
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.reqs) != 1 {
		t.Fatalf("single attempt expected, got %d calls", len(rec.reqs))
	}
}
