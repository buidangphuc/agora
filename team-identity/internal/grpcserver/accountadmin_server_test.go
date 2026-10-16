package grpcserver_test

// Task 8 of identity-user-enforcement: AccountAdminService is registered by Build.
//
// Contract: Build(cfg, auth, addr, session, profile, accountAdmin *handler.AccountAdminHandler, health, logger);
// a nil accountAdmin leaves the service unregistered (Unimplemented).

import (
	"context"
	"io"
	"log/slog"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	identityv1 "github.com/buidangphuc/team-identity/generated/platform/identity/v1"
	"github.com/buidangphuc/team-identity/internal/config"
	"github.com/buidangphuc/team-identity/internal/grpcserver"
	"github.com/buidangphuc/team-identity/internal/handler"
	"github.com/buidangphuc/team-identity/internal/repository"
	"github.com/buidangphuc/team-identity/internal/service"
)

type okAudit struct{}

func (okAudit) WriteUserSetStatus(context.Context, string, string, string, string, string, *time.Time) error {
	return nil
}

func startAdminServer(t *testing.T, withAdmin bool) identityv1.AccountAdminServiceClient {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := &config.Settings{Server: config.Server{Host: "localhost", Port: 0}}
	users := repository.NewInMemoryUserRepository()
	sess := repository.NewInMemorySessionRepository()
	users.AttachSessions(sess)
	svc := service.NewAuthService(users, testSigner(t), time.Hour).WithSessions(sess).WithAudit(okAudit{})
	var admin *handler.AccountAdminHandler
	if withAdmin {
		admin = handler.NewAccountAdminHandler(svc, users, logger)
	}
	srv := grpcserver.Build(cfg, handler.NewAuthHandler(svc), nil, nil, nil, admin, nil, logger)
	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.GracefulStop)
	go func() { _ = srv.Serve(lis) }()
	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
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
	c := startAdminServer(t, true)
	ctx, cancel := mdCtx("admin")
	defer cancel()
	if _, err := c.ListUsers(ctx, &identityv1.ListUsersRequest{}); err != nil {
		t.Fatalf("ListUsers over the wire: %v", err)
	}
}

// User enforcement: Non-admin cannot list users (over gRPC, through the principal interceptor)
func TestAccountAdminServiceRegistered_NonAdminDenied(t *testing.T) {
	c := startAdminServer(t, true)
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

// User enforcement: Non-admin cannot set a status (nil handler leaves the service unregistered)
func TestAccountAdminServiceNilHandlerUnimplemented(t *testing.T) {
	c := startAdminServer(t, false)
	ctx, cancel := mdCtx("admin")
	defer cancel()
	_, err := c.ListUsers(ctx, &identityv1.ListUsersRequest{})
	if status.Code(err) != codes.Unimplemented {
		t.Fatalf("code = %v, want Unimplemented", status.Code(err))
	}
}
