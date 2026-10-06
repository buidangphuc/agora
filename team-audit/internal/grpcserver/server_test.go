package grpcserver_test

import (
	"context"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	auditv1 "github.com/buidangphuc/team-audit/generated/platform/audit/v1"
	"github.com/buidangphuc/team-audit/internal/grpcserver"
	"github.com/buidangphuc/team-audit/internal/handler"
	"github.com/buidangphuc/team-audit/internal/repository"
	"github.com/buidangphuc/team-audit/internal/service"
)

func startServer(t *testing.T) auditv1.AuditServiceClient {
	t.Helper()
	lis := bufconn.Listen(1024 * 1024)
	srv := grpcserver.Build(handler.NewAuditHandler(service.NewAuditService(repository.NewInMemoryAuditRepo())))
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return auditv1.NewAuditServiceClient(conn)
}

// A bare call (no x-principal-* metadata) reaches the handler, which answers
// UNAUTHENTICATED itself; the forwarded principal metadata is resolved by the
// registered interceptor (without it the service write below would also be rejected).
func TestWireAuthorization(t *testing.T) {
	c := startServer(t)
	bare := context.Background()

	if _, err := c.QueryAuditLog(bare, &auditv1.QueryAuditLogRequest{}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("bare query: got %v, want Unauthenticated (handler reached, no principal)", status.Code(err))
	}
	if _, err := c.WriteAuditEvent(bare, &auditv1.WriteAuditEventRequest{Action: "x"}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("bare write: got %v, want Unauthenticated", status.Code(err))
	}

	svc := metadata.NewOutgoingContext(bare, metadata.Pairs(
		"x-principal-id", "service-x", "x-principal-type", "service", "x-principal-scopes", "audit.write"))
	if _, err := c.WriteAuditEvent(svc, &auditv1.WriteAuditEventRequest{ActorId: "u1", Action: "login", TargetType: "user"}); err != nil {
		t.Fatalf("service write over the wire: %v", err)
	}
	adm := metadata.NewOutgoingContext(bare, metadata.Pairs(
		"x-principal-id", "admin-1", "x-principal-type", "user", "x-principal-scopes", "admin"))
	res, err := c.QueryAuditLog(adm, &auditv1.QueryAuditLogRequest{})
	if err != nil || len(res.GetEvents()) != 1 {
		t.Fatalf("admin query over the wire: %+v %v", res, err)
	}
}
