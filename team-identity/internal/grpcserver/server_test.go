package grpcserver_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
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
	"github.com/buidangphuc/team-identity/internal/token"
)

func testSigner(t *testing.T) *token.Signer {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	s, err := token.NewSigner(string(pemBytes), "test-kid")
	if err != nil {
		t.Fatalf("NewSigner: %v", err)
	}
	return s
}

func startServer(t *testing.T, userRepo repository.UserRepository, addrRepo repository.AddressRepository) (identityv1.AuthServiceClient, identityv1.AddressServiceClient) {
	authClient, addrClient, _ := startServerWithSessions(t, userRepo, addrRepo, repository.NewInMemorySessionRepository())
	return authClient, addrClient
}

func startServerWithSessions(
	t *testing.T,
	userRepo repository.UserRepository,
	addrRepo repository.AddressRepository,
	sessionRepo repository.SessionRepository,
) (identityv1.AuthServiceClient, identityv1.AddressServiceClient, identityv1.SessionServiceClient) {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := &config.Settings{
		Server: config.Server{Host: "localhost", Port: 0},
		JWT:    config.JWT{KID: "test-kid", JWKSHTTPPort: 50063, TTLSeconds: 3600},
	}
	authSvc := service.NewAuthService(userRepo, testSigner(t), time.Hour)
	authHandler := handler.NewAuthHandler(authSvc)
	addrHandler := handler.NewAddressHandler(addrRepo, logger)
	sessionHandler := handler.NewSessionHandler(sessionRepo, logger)

	srv := grpcserver.Build(cfg, authHandler, addrHandler, sessionHandler, nil, logger)

	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(srv.GracefulStop)

	go func() {
		_ = srv.Serve(lis)
	}()

	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	return identityv1.NewAuthServiceClient(conn),
		identityv1.NewAddressServiceClient(conn),
		identityv1.NewSessionServiceClient(conn)
}

func principalCtx(t *testing.T, userID string) (context.Context, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	md := metadata.Pairs(
		"x-principal-id", userID,
		"x-principal-type", "user",
		"x-principal-scopes", "identity.read,identity.write",
	)
	return metadata.NewOutgoingContext(ctx, md), cancel
}

func TestAddressService_CRUD_And_Default(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	addrRepo := repository.NewInMemoryAddressRepository()
	_, addrClient := startServer(t, userRepo, addrRepo)

	ctx, cancel := principalCtx(t, "user-buyer-1")
	defer cancel()

	// 1. Create first address
	resp1, err := addrClient.CreateAddress(ctx, &identityv1.CreateAddressRequest{
		RecipientName: "Nguyen Van A",
		Phone:         "0901234567",
		Street:        "123 Nguyen Hue",
		District:      "Quan 1",
		City:          "TP Ho Chi Minh",
		IsDefault:     false,
	})
	if err != nil {
		t.Fatalf("CreateAddress 1: %v", err)
	}
	if resp1.GetAddress().GetId() == "" {
		t.Fatal("expected non-empty address id")
	}

	// 2. Create second address as default
	resp2, err := addrClient.CreateAddress(ctx, &identityv1.CreateAddressRequest{
		RecipientName: "Nguyen Van A (VP)",
		Phone:         "0909888777",
		Street:        "456 Le Loi",
		District:      "Quan 1",
		City:          "TP Ho Chi Minh",
		IsDefault:     true,
	})
	if err != nil {
		t.Fatalf("CreateAddress 2: %v", err)
	}
	if !resp2.GetAddress().GetIsDefault() {
		t.Fatal("expected address 2 to be default")
	}

	// 3. List addresses
	listResp, err := addrClient.ListAddresses(ctx, &identityv1.ListAddressesRequest{})
	if err != nil {
		t.Fatalf("ListAddresses: %v", err)
	}
	if len(listResp.GetAddresses()) != 2 {
		t.Fatalf("want 2 addresses, got %d", len(listResp.GetAddresses()))
	}

	// 4. Set first address as default
	setDefaultResp, err := addrClient.SetDefaultAddress(ctx, &identityv1.SetDefaultAddressRequest{
		Id: resp1.GetAddress().GetId(),
	})
	if err != nil {
		t.Fatalf("SetDefaultAddress: %v", err)
	}
	if !setDefaultResp.GetAddress().GetIsDefault() {
		t.Fatal("expected address 1 to be marked default")
	}

	// 5. Delete second address
	_, err = addrClient.DeleteAddress(ctx, &identityv1.DeleteAddressRequest{
		Id: resp2.GetAddress().GetId(),
	})
	if err != nil {
		t.Fatalf("DeleteAddress: %v", err)
	}

	// Verify count is 1
	listRespAfter, err := addrClient.ListAddresses(ctx, &identityv1.ListAddressesRequest{})
	if err != nil {
		t.Fatalf("ListAddresses after delete: %v", err)
	}
	if len(listRespAfter.GetAddresses()) != 1 {
		t.Fatalf("want 1 address, got %d", len(listRespAfter.GetAddresses()))
	}
}

func TestPasswordManagement_E2E(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	addrRepo := repository.NewInMemoryAddressRepository()
	authClient, _ := startServer(t, userRepo, addrRepo)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 1. Register a user
	regResp, err := authClient.Register(ctx, &identityv1.RegisterRequest{
		Username: "pwd_user",
		Password: "oldpassword123",
		Role:     "buyer",
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	userID := regResp.GetResult().GetPrincipal().GetId()

	// 2. Change password
	cpResp, err := authClient.ChangePassword(ctx, &identityv1.ChangePasswordRequest{
		UserId:      userID,
		OldPassword: "oldpassword123",
		NewPassword: "newpassword456",
	})
	if err != nil {
		t.Fatalf("ChangePassword: %v", err)
	}
	if !cpResp.GetSuccess() {
		t.Fatal("expected ChangePassword success=true")
	}

	// 3. Login with new password
	loginResp, err := authClient.Login(ctx, &identityv1.LoginRequest{
		Username: "pwd_user",
		Password: "newpassword456",
	})
	if err != nil {
		t.Fatalf("Login with new password: %v", err)
	}
	if loginResp.GetResult().GetUsername() != "pwd_user" {
		t.Fatalf("want pwd_user, got %s", loginResp.GetResult().GetUsername())
	}

	// 4. Request password reset
	reqResetResp, err := authClient.RequestPasswordReset(ctx, &identityv1.RequestPasswordResetRequest{
		Username: "pwd_user",
	})
	if err != nil {
		t.Fatalf("RequestPasswordReset: %v", err)
	}
	if reqResetResp.GetResetToken() == "" {
		t.Fatal("expected non-empty reset token")
	}

	// 5. Reset password
	resetResp, err := authClient.ResetPassword(ctx, &identityv1.ResetPasswordRequest{
		Token:       reqResetResp.GetResetToken(),
		NewPassword: "resetpassword789",
	})
	if err != nil {
		t.Fatalf("ResetPassword: %v", err)
	}
	if !resetResp.GetSuccess() {
		t.Fatal("expected ResetPassword success=true")
	}

	// 6. Login with reset password
	loginResetResp, err := authClient.Login(ctx, &identityv1.LoginRequest{
		Username: "pwd_user",
		Password: "resetpassword789",
	})
	if err != nil {
		t.Fatalf("Login with reset password: %v", err)
	}
	if loginResetResp.GetResult().GetUsername() != "pwd_user" {
		t.Fatalf("want pwd_user, got %s", loginResetResp.GetResult().GetUsername())
	}
}

func TestSessionService_GRPC_SelfScoped(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	addrRepo := repository.NewInMemoryAddressRepository()
	sessionRepo := repository.NewInMemorySessionRepository()

	// Seed sessions + login history for two distinct users.
	ctxBg := context.Background()
	sA, err := sessionRepo.CreateSession(ctxBg, repository.Session{UserID: "user-A", Device: "iPhone", IP: "10.0.0.1"})
	if err != nil {
		t.Fatalf("seed session A: %v", err)
	}
	sB, err := sessionRepo.CreateSession(ctxBg, repository.Session{UserID: "user-B", Device: "Pixel", IP: "10.0.0.2"})
	if err != nil {
		t.Fatalf("seed session B: %v", err)
	}
	if _, err := sessionRepo.RecordLogin(ctxBg, repository.LoginEvent{UserID: "user-A", IP: "10.0.0.1", UserAgent: "curl", Success: true}); err != nil {
		t.Fatalf("seed login A: %v", err)
	}

	_, _, sessionClient := startServerWithSessions(t, userRepo, addrRepo, sessionRepo)

	ctxA, cancelA := principalCtx(t, "user-A")
	defer cancelA()

	// user-A sees only their own session.
	listA, err := sessionClient.ListSessions(ctxA, &identityv1.ListSessionsRequest{})
	if err != nil {
		t.Fatalf("ListSessions A: %v", err)
	}
	if len(listA.GetSessions()) != 1 || listA.GetSessions()[0].GetId() != sA.ID {
		t.Fatalf("want 1 session (%s) for user-A, got %+v", sA.ID, listA.GetSessions())
	}

	// user-A cannot revoke user-B's session (cross-user isolation).
	if _, err := sessionClient.RevokeSession(ctxA, &identityv1.RevokeSessionRequest{SessionId: sB.ID}); status.Code(err) != codes.NotFound {
		t.Fatalf("want NotFound revoking cross-user session, got %v", err)
	}

	// user-A revokes their own session.
	if _, err := sessionClient.RevokeSession(ctxA, &identityv1.RevokeSessionRequest{SessionId: sA.ID}); err != nil {
		t.Fatalf("RevokeSession own: %v", err)
	}

	// Login history is self-scoped: user-B has none.
	ctxB, cancelB := principalCtx(t, "user-B")
	defer cancelB()
	histB, err := sessionClient.ListLoginHistory(ctxB, &identityv1.ListLoginHistoryRequest{})
	if err != nil {
		t.Fatalf("ListLoginHistory B: %v", err)
	}
	if len(histB.GetEvents()) != 0 || histB.GetPage().GetTotal() != 0 {
		t.Fatalf("want empty login history for user-B, got %+v", histB)
	}

	// Anonymous (no principal) is rejected, not panicked.
	anonCtx, cancelAnon := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancelAnon()
	if _, err := sessionClient.ListSessions(anonCtx, &identityv1.ListSessionsRequest{}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("want Unauthenticated for anonymous, got %v", err)
	}
}
