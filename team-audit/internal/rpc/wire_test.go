package rpc_test

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	reflectionpb "google.golang.org/grpc/reflection/grpc_reflection_v1"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
	auditv1 "github.com/buidangphuc/team-audit/generated/platform/audit/v1"
)

// Scenario: The forwarded principal metadata is honoured over the wire
func TestWireAuthorization(t *testing.T) {
	h, _ := newHandler(t)
	c := dial(t, h, quiet())
	bare := context.Background()

	if _, err := c.QueryAuditLog(bare, &auditv1.QueryAuditLogRequest{}); code(err) != codes.Unauthenticated {
		t.Fatalf("bare query: got %v, want Unauthenticated", code(err))
	}
	if _, err := c.WriteAuditEvent(bare, &auditv1.WriteAuditEventRequest{Action: "x"}); code(err) != codes.Unauthenticated {
		t.Fatalf("bare write: got %v, want Unauthenticated", code(err))
	}
	svc := wire("x-principal-id", "service-x", "x-principal-type", "service", "x-principal-scopes", "audit.write")
	if _, err := c.WriteAuditEvent(svc, &auditv1.WriteAuditEventRequest{ActorId: "u1", Action: "login", TargetType: "user"}); err != nil {
		t.Fatalf("service write over the wire: %v", err)
	}
	res, err := c.QueryAuditLog(wireAdmin(), &auditv1.QueryAuditLogRequest{})
	if err != nil || len(res.GetEvents()) != 1 || res.GetEvents()[0].GetActorId() != "u1" {
		t.Fatalf("admin query over the wire: %+v %v", res, err)
	}
}

// Scenario: Reflection is not served by default
func TestReflectionOffByDefault(t *testing.T) {
	h, _ := newHandler(t)
	conn := startServer(t, h, quiet(), config.GRPC{})
	stream, err := reflectionpb.NewServerReflectionClient(conn).ServerReflectionInfo(context.Background())
	if err == nil {
		err = stream.Send(&reflectionpb.ServerReflectionRequest{
			MessageRequest: &reflectionpb.ServerReflectionRequest_ListServices{ListServices: ""},
		})
	}
	if err == nil {
		_, err = stream.Recv()
	}
	if code(err) != codes.Unimplemented {
		t.Fatalf("reflection: got %v, want Unimplemented", err)
	}
}
