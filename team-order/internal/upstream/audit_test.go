package upstream

import (
	"context"
	"io"
	"log/slog"
	"net"
	"os"
	"reflect"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auditx"
	sdkauth "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	sdkconfig "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"

	auditv1 "github.com/buidangphuc/team-order/generated/platform/audit/v1"
)

type recordingAuditServer struct {
	auditv1.UnimplementedAuditServiceServer
	mu       sync.Mutex
	md       []metadata.MD
	reqs     []*auditv1.WriteAuditEventRequest
	blocking bool
}

func (s *recordingAuditServer) WriteAuditEvent(ctx context.Context, r *auditv1.WriteAuditEventRequest) (*auditv1.WriteAuditEventResponse, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	s.mu.Lock()
	s.md = append(s.md, md)
	s.reqs = append(s.reqs, r)
	s.mu.Unlock()
	if s.blocking {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return &auditv1.WriteAuditEventResponse{}, nil
}

func (s *recordingAuditServer) attempts() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.md)
}

func startAuditServer(t *testing.T, srv *recordingAuditServer) string {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	g := grpc.NewServer()
	auditv1.RegisterAuditServiceServer(g, srv)
	go func() { _ = g.Serve(lis) }()
	t.Cleanup(g.Stop)
	return lis.Addr().String()
}

func newAuditWriter(t *testing.T, addr string) *AuditWriter {
	t.Helper()
	conn, err := auditx.Dial(sdkconfig.Audit{Addr: addr}, "service-team-order", "audit.write")
	if err != nil || conn == nil {
		t.Fatalf("auditx.Dial: conn=%v err=%v", conn, err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	w := NewAuditWriter(auditv1.NewAuditServiceClient(conn))
	if w == nil {
		t.Fatal("NewAuditWriter returned nil")
	}
	return w
}

// Scenario: Order audit: Audit call carries the service principal only
func TestAuditWriterUsesServicePrincipal(t *testing.T) {
	srv := &recordingAuditServer{}
	w := newAuditWriter(t, startAuditServer(t, srv))

	adminMD := metadata.Pairs(
		"x-principal-id", "admin-1",
		"x-principal-type", "user",
		"x-principal-scopes", "admin,order.admin",
		"authorization", "Bearer admin-token",
	)
	ctx := metadata.NewIncomingContext(context.Background(), adminMD)
	ctx = sdkauth.WithPrincipal(ctx, sdkauth.Principal{ID: "admin-1", Type: sdkauth.TypeUser, Scopes: []string{"admin", "order.admin"}})
	meta := map[string]string{"fail_step": "payment"}
	ev := auditx.Event{ActorID: "admin-1", Action: "order.force_fail", TargetType: "order", TargetID: "ord_1", Metadata: meta}
	if err := w.WriteAuditEvent(ctx, ev); err != nil {
		t.Fatalf("WriteAuditEvent: %v", err)
	}
	if srv.attempts() != 1 {
		t.Fatalf("attempts = %d, want 1", srv.attempts())
	}
	md := srv.md[0]
	for k, want := range map[string]string{
		"x-principal-id":     "service-team-order",
		"x-principal-type":   "service",
		"x-principal-scopes": "audit.write",
	} {
		if v := md.Get(k); len(v) != 1 || v[0] != want {
			t.Fatalf("%s = %v, want exactly [%s]", k, v, want)
		}
	}
	if v := md.Get("authorization"); len(v) != 0 {
		t.Fatalf("admin authorization leaked: %v", v)
	}
	r := srv.reqs[0]
	if r.GetActorId() != "admin-1" || r.GetAction() != "order.force_fail" || r.GetTargetType() != "order" ||
		r.GetTargetId() != "ord_1" || !reflect.DeepEqual(r.GetMetadata(), meta) {
		t.Fatalf("request not carried through: %v", r)
	}
}

// Scenario: Order audit: Audit call is bounded by the timeout
func TestAuditWriterTimeoutNoRetry(t *testing.T) {
	srv := &recordingAuditServer{blocking: true}
	w := newAuditWriter(t, startAuditServer(t, srv))
	gate := auditx.NewGate(w, slog.New(slog.NewTextHandler(io.Discard, nil)))

	ran := false
	start := time.Now()
	err := gate.Do(context.Background(), auditx.Event{ActorID: "admin-1", Action: "order.force_fail", TargetType: "order", TargetID: "ord_1"},
		func(context.Context) error { ran = true; return nil })
	elapsed := time.Since(start)
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("err = %v, want Unavailable", err)
	}
	if ran {
		t.Fatal("action ran although the audit write did not complete")
	}
	if elapsed > 3*time.Second {
		t.Fatalf("took %v, want within 3s", elapsed)
	}
	if n := srv.attempts(); n != 1 {
		t.Fatalf("attempts = %d, want exactly 1 (no retry)", n)
	}
}

// Scenario: Order audit: Audit proto copy is current
func TestAuditProtoCopyMatchesCore(t *testing.T) {
	const rel = "platform/audit/v1/audit.proto"
	core, err := os.ReadFile("../../../platform-core/packages/proto/" + rel)
	if err != nil {
		t.Skipf("platform-core not available alongside team-order: %v", err)
	}
	local, err := os.ReadFile("../../proto/" + rel)
	if err != nil {
		t.Fatalf("team-order proto copy missing: %v", err)
	}
	if string(core) != string(local) {
		t.Fatal("team-order audit.proto differs from platform-core/packages/proto")
	}
}
