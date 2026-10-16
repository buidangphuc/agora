package upstream

import (
	"context"
	"net"
	"os"
	"reflect"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

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

// Scenario: Order audit: Audit call carries the service principal only
func TestAuditClientUsesServicePrincipal(t *testing.T) {
	srv := &recordingAuditServer{}
	c, err := NewAuditClient(startAuditServer(t, srv), 2*time.Second)
	if err != nil || c == nil {
		t.Fatalf("NewAuditClient: client=%v err=%v", c, err)
	}
	defer c.Close()

	admin := metadata.Pairs(
		"x-principal-id", "admin-1",
		"x-principal-type", "user",
		"x-principal-scopes", "admin,order.admin",
		"authorization", "Bearer admin-token",
	)
	ctx := metadata.NewOutgoingContext(metadata.NewIncomingContext(context.Background(), admin), admin)
	meta := map[string]string{"fail_step": "payment"}
	if err := c.WriteAdminEvent(ctx, "admin-1", "order.force_fail", "order", "ord_1", meta); err != nil {
		t.Fatalf("WriteAdminEvent: %v", err)
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
func TestAuditClientTimeoutNoRetry(t *testing.T) {
	srv := &recordingAuditServer{blocking: true}
	timeout := 200 * time.Millisecond
	c, err := NewAuditClient(startAuditServer(t, srv), timeout)
	if err != nil || c == nil {
		t.Fatalf("NewAuditClient: client=%v err=%v", c, err)
	}
	defer c.Close()

	start := time.Now()
	err = c.WriteAdminEvent(context.Background(), "admin-1", "order.force_fail", "order", "ord_1", nil)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("want an error when team-audit does not answer")
	}
	if elapsed > timeout+500*time.Millisecond {
		t.Fatalf("took %v, want within timeout+500ms", elapsed)
	}
	if n := srv.attempts(); n != 1 {
		t.Fatalf("attempts = %d, want exactly 1 (no retry)", n)
	}
}

// Scenario: Order audit: Unset audit address yields no client
func TestNewAuditClientEmptyAddrNil(t *testing.T) {
	c, err := NewAuditClient("", time.Second)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if c != nil {
		t.Fatalf("client = %v, want nil", c)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("Close on nil client = %v, want nil", err)
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
