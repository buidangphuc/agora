package handler_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"testing"
	"time"

	identityv1 "github.com/buidangphuc/team-identity/generated/platform/identity/v1"
	"github.com/buidangphuc/team-identity/internal/handler"
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

		cpRes, err := h.ChangePassword(ctx, &identityv1.ChangePasswordRequest{
			UserId:      reg.Result.Principal.Id,
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
