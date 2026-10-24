package audit_test

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

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auditx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
	auditv1 "github.com/buidangphuc/team-domain/generated/platform/audit/v1"
	"github.com/buidangphuc/team-domain/internal/audit"
)

var _ auditx.Writer = (*audit.Writer)(nil)

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

func (r *recordingAudit) calls() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.reqs)
}

func startAudit(t *testing.T, svc *recordingAudit) *audit.Writer {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := grpc.NewServer()
	auditv1.RegisterAuditServiceServer(srv, svc)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	conn, err := auditx.Dial(config.Audit{Addr: lis.Addr().String()}, "service-team-domain", "audit.write")
	if err != nil || conn == nil {
		t.Fatalf("auditx.Dial: %v, %v", conn, err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return audit.NewWriter(auditv1.NewAuditServiceClient(conn))
}

func updateEvent() auditx.Event {
	return auditx.Event{
		ActorID: "adm-1", Action: "listing.admin_update", TargetType: "listing", TargetID: "L-1",
		Metadata: map[string]string{"seller_id": "s-1", "status": "rejected"},
	}
}

func callerCtx(t *testing.T) context.Context {
	t.Helper()
	ctx := auth.WithPrincipal(t.Context(), auth.Principal{ID: "seller-9", Type: auth.TypeUser, Scopes: []string{"listing.write", "listing.read"}})
	return metadata.NewOutgoingContext(ctx, metadata.Pairs(
		"x-principal-id", "seller-9", "x-principal-type", "user", "x-principal-scopes", "listing.write,listing.read"))
}

// Scenario: Listing audit: Audit is called as the service principal.
func TestAuditWriterUsesServicePrincipal(t *testing.T) {
	rec := &recordingAudit{}
	w := startAudit(t, rec)

	events := []auditx.Event{
		updateEvent(),
		{ActorID: "adm-1", Action: "listing.admin_delete", TargetType: "listing", TargetID: "L-1", Metadata: map[string]string{"seller_id": "s-1"}},
		{ActorID: "adm-1", Action: "listing.moderate", TargetType: "listing", TargetID: "L-1", Metadata: map[string]string{
			"seller_id": "s-1", "decision": "reject", "reason_code": "counterfeit", "note": "Fake logo",
			"from_status": "pending_review", "to_status": "rejected",
		}},
	}
	for _, ev := range events {
		if err := w.WriteAuditEvent(callerCtx(t), ev); err != nil {
			t.Fatalf("WriteAuditEvent(%s): %v", ev.Action, err)
		}
	}
	if rec.calls() != len(events) {
		t.Fatalf("want %d WriteAuditEvent calls, got %d", len(events), rec.calls())
	}
	for i, ev := range events {
		got := rec.reqs[i]
		if got.GetActorId() != ev.ActorID || got.GetAction() != ev.Action || got.GetTargetType() != "listing" || got.GetTargetId() != ev.TargetID {
			t.Errorf("event %d = %+v, want actor %s action %s target listing/%s", i, got, ev.ActorID, ev.Action, ev.TargetID)
		}
		if len(got.GetMetadata()) != len(ev.Metadata) {
			t.Errorf("event %d metadata = %v, want %v", i, got.GetMetadata(), ev.Metadata)
		}
		for k, v := range ev.Metadata {
			if got.GetMetadata()[k] != v {
				t.Errorf("event %d metadata[%s] = %q, want %q", i, k, got.GetMetadata()[k], v)
			}
		}
	}
	want := map[string]string{
		"x-principal-id":     "service-team-domain",
		"x-principal-type":   "service",
		"x-principal-scopes": "audit.write",
	}
	for i, md := range rec.mds {
		for k, v := range want {
			if vals := md.Get(k); len(vals) != 1 || vals[0] != v {
				t.Errorf("call %d %s = %v, want [%s]", i, k, vals, v)
			}
		}
	}
}

// Scenario: Listing audit: Slow audit is cut off at the timeout.
func TestAuditGateTimeoutSingleAttempt(t *testing.T) {
	cases := []struct {
		name string
		rec  *recordingAudit
	}{
		{"error response", &recordingAudit{err: status.Error(codes.Internal, "audit db down")}},
		{"hung audit", &recordingAudit{block: true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gate := auditx.NewGate(startAudit(t, tc.rec), nil)
			ran := false
			start := time.Now()
			err := gate.Do(t.Context(), updateEvent(), func(context.Context) error { ran = true; return nil })
			if status.Code(err) != codes.Unavailable || status.Convert(err).Message() != auditx.Message {
				t.Fatalf("err = %v, want Unavailable %q", err, auditx.Message)
			}
			if ran {
				t.Fatal("the action must not run when the audit write fails")
			}
			if el := time.Since(start); el > 3*time.Second {
				t.Fatalf("call took %v, want a bounded single attempt", el)
			}
			if n := tc.rec.calls(); n != 1 {
				t.Fatalf("want exactly 1 attempt (no retry), got %d", n)
			}
		})
	}
}

// Scenario: Listing audit: Caller principal is never forwarded.
func TestAuditWriterNeverForwardsCallerPrincipal(t *testing.T) {
	rec := &recordingAudit{}
	w := startAudit(t, rec)
	ev := auditx.Event{ActorID: "adm-1", Action: "listing.admin_delete", TargetType: "listing", TargetID: "L-1", Metadata: map[string]string{"seller_id": "s-1"}}
	if err := w.WriteAuditEvent(callerCtx(t), ev); err != nil {
		t.Fatalf("WriteAuditEvent: %v", err)
	}
	if rec.calls() != 1 {
		t.Fatalf("want 1 call, got %d", rec.calls())
	}
	for k, vals := range rec.mds[0] {
		for _, v := range vals {
			if v == "seller-9" || v == "listing.write,listing.read" || v == "user" {
				t.Errorf("caller principal leaked in %s=%v", k, vals)
			}
		}
	}
}
