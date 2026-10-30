package rpc_test

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	identityv1 "github.com/buidangphuc/team-identity/generated/platform/identity/v1"
	"github.com/buidangphuc/team-identity/internal/rpc"
	"github.com/buidangphuc/team-identity/internal/store/storetest"
)

func startAdminServer(t *testing.T) identityv1.AccountAdminServiceClient {
	t.Helper()
	logger := discardLogger()
	users := storetest.NewInMemoryUserRepository()
	sess := storetest.NewInMemorySessionRepository()
	users.AttachSessions(sess)
	svc := newSvc(t, users, sess)
	conn := startWire(t, rpc.Handlers{
		Auth:    rpc.NewAuthHandler(svc),
		Address: rpc.NewAddressHandler(storetest.NewInMemoryAddressRepository(), logger),
		Session: rpc.NewSessionHandler(sess, logger),
		Profile: rpc.NewProfileHandler(users, logger),
		Admin:   rpc.NewAccountAdminHandler(svc, users, logger),
	}, logger)
	return identityv1.NewAccountAdminServiceClient(conn)
}

func mdCtx(scopes string) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	return metadata.NewOutgoingContext(ctx, metadata.Pairs(
		"x-principal-id", "admin-1", "x-principal-type", "user", "x-principal-scopes", scopes,
	)), cancel
}

// User enforcement: Listing never exposes credentials and is not audited (served over gRPC for an admin)
func TestAccountAdminServiceRegistered_AdminCanList(t *testing.T) {
	c := startAdminServer(t)
	ctx, cancel := mdCtx("admin")
	defer cancel()
	if _, err := c.ListUsers(ctx, &identityv1.ListUsersRequest{}); err != nil {
		t.Fatalf("ListUsers over the wire: %v", err)
	}
}

// User enforcement: Non-admin cannot list users (over gRPC, through the principal interceptor)
func TestAccountAdminServiceRegistered_NonAdminDenied(t *testing.T) {
	c := startAdminServer(t)
	ctx, cancel := mdCtx("listing.write")
	defer cancel()
	_, err := c.ListUsers(ctx, &identityv1.ListUsersRequest{})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("code = %v (%v), want PermissionDenied", status.Code(err), err)
	}
	_, err = c.SetUserStatus(ctx, &identityv1.SetUserStatusRequest{UserId: "u", Status: identityv1.UserStatus_USER_STATUS_BANNED, ReasonCode: "scam"})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("SetUserStatus code = %v (%v), want PermissionDenied", status.Code(err), err)
	}
}
