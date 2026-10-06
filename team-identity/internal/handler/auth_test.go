package handler_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	commonv1 "github.com/buidangphuc/team-identity/generated/platform/common/v1"
	identityv1 "github.com/buidangphuc/team-identity/generated/platform/identity/v1"
	"github.com/buidangphuc/team-identity/internal/handler"
	"github.com/buidangphuc/team-identity/internal/interceptor"
	"github.com/buidangphuc/team-identity/internal/repository"
	"github.com/buidangphuc/team-identity/internal/service"
	"github.com/buidangphuc/team-identity/internal/token"
)

// testSigner builds an RS256 signer from a throwaway RSA keypair (ADR-0006).
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

func userPrincipal(id string) *commonv1.Principal {
	return &commonv1.Principal{Id: id, Type: commonv1.PrincipalType_PRINCIPAL_TYPE_USER}
}

func TestChangePassword_PrincipalGate(t *testing.T) {
	repo := repository.NewInMemoryUserRepository()
	svc := service.NewAuthService(repo, testSigner(t), time.Hour)
	h := handler.NewAuthHandler(svc)
	bg := context.Background()

	victim, err := h.Register(bg, &identityv1.RegisterRequest{Username: "victim", Password: "victimpass1", Role: "buyer"})
	if err != nil {
		t.Fatal(err)
	}
	attacker, err := h.Register(bg, &identityv1.RegisterRequest{Username: "attacker", Password: "attackerpass1", Role: "buyer"})
	if err != nil {
		t.Fatal(err)
	}
	victimLogin := func(pw string) error {
		_, err := h.Login(bg, &identityv1.LoginRequest{Username: "victim", Password: pw})
		return err
	}

	t.Run("anonymous denied", func(t *testing.T) {
		_, err := h.ChangePassword(bg, &identityv1.ChangePasswordRequest{UserId: victim.Result.Principal.Id, OldPassword: "victimpass1", NewPassword: "hacked-pass"})
		if status.Code(err) != codes.Unauthenticated {
			t.Fatalf("want Unauthenticated, got %v", err)
		}
		anon := interceptor.ContextWithPrincipal(bg, &commonv1.Principal{Id: "anonymous", Type: commonv1.PrincipalType_PRINCIPAL_TYPE_ANONYMOUS})
		_, err = h.ChangePassword(anon, &identityv1.ChangePasswordRequest{UserId: victim.Result.Principal.Id, OldPassword: "victimpass1", NewPassword: "hacked-pass"})
		if status.Code(err) != codes.Unauthenticated {
			t.Fatalf("anonymous principal: want Unauthenticated, got %v", err)
		}
	})

	t.Run("service principal denied", func(t *testing.T) {
		ctx := interceptor.ContextWithPrincipal(bg, &commonv1.Principal{Id: "svc-x", Type: commonv1.PrincipalType_PRINCIPAL_TYPE_SERVICE})
		_, err := h.ChangePassword(ctx, &identityv1.ChangePasswordRequest{OldPassword: "victimpass1", NewPassword: "hacked-pass"})
		if status.Code(err) != codes.PermissionDenied {
			t.Fatalf("want PermissionDenied, got %v", err)
		}
	})

	t.Run("body user_id of another user is ignored", func(t *testing.T) {
		ctx := interceptor.ContextWithPrincipal(bg, userPrincipal(attacker.Result.Principal.Id))
		// Attacker knows the victim's old password but targets via body user_id:
		// the call applies to the attacker (whose old password differs) and fails.
		_, err := h.ChangePassword(ctx, &identityv1.ChangePasswordRequest{UserId: victim.Result.Principal.Id, OldPassword: "victimpass1", NewPassword: "hacked-pass"})
		if status.Code(err) != codes.Unauthenticated {
			t.Fatalf("want Unauthenticated (old password checked against principal), got %v", err)
		}
		if err := victimLogin("victimpass1"); err != nil {
			t.Fatalf("victim password must be unchanged: %v", err)
		}
		// With the attacker's own old password it changes the attacker, not the victim.
		if _, err := h.ChangePassword(ctx, &identityv1.ChangePasswordRequest{UserId: victim.Result.Principal.Id, OldPassword: "attackerpass1", NewPassword: "attacker-new1"}); err != nil {
			t.Fatal(err)
		}
		if err := victimLogin("victimpass1"); err != nil {
			t.Fatalf("victim password must be unchanged: %v", err)
		}
	})

	t.Run("owner allowed", func(t *testing.T) {
		ctx := interceptor.ContextWithPrincipal(bg, userPrincipal(victim.Result.Principal.Id))
		if _, err := h.ChangePassword(ctx, &identityv1.ChangePasswordRequest{OldPassword: "victimpass1", NewPassword: "victimpass2"}); err != nil {
			t.Fatal(err)
		}
		if err := victimLogin("victimpass2"); err != nil {
			t.Fatalf("login with new password: %v", err)
		}
	})
}

func TestAuthHandler(t *testing.T) {
	repo := repository.NewInMemoryUserRepository()
	svc := service.NewAuthService(repo, testSigner(t), time.Hour)
	h := handler.NewAuthHandler(svc)
	ctx := context.Background()

	t.Run("Register handler", func(t *testing.T) {
		res, err := h.Register(ctx, &identityv1.RegisterRequest{
			Username: "handler_user",
			Password: "password123",
			Role:     "buyer",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Result.Username != "handler_user" {
			t.Errorf("expected handler_user, got %s", res.Result.Username)
		}
	})

	t.Run("Login handler", func(t *testing.T) {
		res, err := h.Login(ctx, &identityv1.LoginRequest{
			Username: "handler_user",
			Password: "password123",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Result.Username != "handler_user" {
			t.Errorf("expected handler_user, got %s", res.Result.Username)
		}
	})

	t.Run("ChangePassword handler", func(t *testing.T) {
		reg, err := h.Register(ctx, &identityv1.RegisterRequest{
			Username: "cp_handler_user",
			Password: "oldpassword123",
			Role:     "buyer",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		cpRes, err := h.ChangePassword(interceptor.ContextWithPrincipal(ctx, userPrincipal(reg.Result.Principal.Id)), &identityv1.ChangePasswordRequest{
			OldPassword: "oldpassword123",
			NewPassword: "newpassword456",
		})
		if err != nil {
			t.Fatalf("unexpected error changing password: %v", err)
		}
		if !cpRes.Success {
			t.Errorf("expected success to be true")
		}

		// Login with new password
		loginRes, err := h.Login(ctx, &identityv1.LoginRequest{
			Username: "cp_handler_user",
			Password: "newpassword456",
		})
		if err != nil {
			t.Fatalf("unexpected error logging in with new password: %v", err)
		}
		if loginRes.Result.Username != "cp_handler_user" {
			t.Errorf("expected cp_handler_user, got %s", loginRes.Result.Username)
		}
	})

	t.Run("RequestPasswordReset and ResetPassword handler", func(t *testing.T) {
		_, err := h.Register(ctx, &identityv1.RegisterRequest{
			Username: "reset_handler_user",
			Password: "initialpassword123",
			Role:     "buyer",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		reqResetRes, err := h.RequestPasswordReset(ctx, &identityv1.RequestPasswordResetRequest{
			Username: "reset_handler_user",
		})
		if err != nil {
			t.Fatalf("unexpected error requesting reset: %v", err)
		}
		if reqResetRes.ResetToken == "" {
			t.Errorf("expected non-empty reset token")
		}
		if reqResetRes.ExpiresAt <= 0 {
			t.Errorf("expected valid expires_at timestamp")
		}

		resetRes, err := h.ResetPassword(ctx, &identityv1.ResetPasswordRequest{
			Token:       reqResetRes.ResetToken,
			NewPassword: "newlyresetpassword999",
		})
		if err != nil {
			t.Fatalf("unexpected error resetting password: %v", err)
		}
		if !resetRes.Success {
			t.Errorf("expected success to be true")
		}

		// Login with new password
		loginRes, err := h.Login(ctx, &identityv1.LoginRequest{
			Username: "reset_handler_user",
			Password: "newlyresetpassword999",
		})
		if err != nil {
			t.Fatalf("unexpected error logging in with new password: %v", err)
		}
		if loginRes.Result.Username != "reset_handler_user" {
			t.Errorf("expected reset_handler_user, got %s", loginRes.Result.Username)
		}
	})
}
