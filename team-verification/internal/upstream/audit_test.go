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

func dialWriter(t *testing.T, addr string) auditx.Writer {
	t.Helper()
	conn, err := auditx.Dial(config.Audit{Addr: addr}, "service-team-verification", "audit.write")
	if err != nil || conn == nil {
		t.Fatalf("auditx.Dial: %v, %v", conn, err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return NewAuditWriter(conn)
}

// Scenario: Audit call uses the service principal.
func TestAuditClientUsesServicePrincipal(t *testing.T) {
	rec := &recordingAudit{}
	w := dialWriter(t, startAudit(t, rec))

	ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs(
		"x-principal-id", "adm-1", "x-principal-type", "user", "x-principal-scopes", "admin"))
	ev := auditx.Event{
		ActorID: "adm-1", Action: "kyc.review", TargetType: "kyc_submission", TargetID: "kyc_1",
		Metadata: map[string]string{"decision": "approve", "user_id": "u-1"},
	}
	if err := w.WriteAuditEvent(ctx, ev); err != nil {
		t.Fatalf("WriteAuditEvent: %v", err)
	}

	rec.mu.Lock()
	defer rec.mu.Unlock()
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
	w := dialWriter(t, startAudit(t, &recordingAudit{block: true}))
	gate := auditx.NewGate(w, slog.New(slog.DiscardHandler))
	applied := false
	start := time.Now()
	err := gate.Do(context.Background(), auditx.Event{
		ActorID: "adm-1", Action: "kyc.review", TargetType: "kyc_submission", TargetID: "kyc_1",
	}, func(context.Context) error { applied = true; return nil })
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
