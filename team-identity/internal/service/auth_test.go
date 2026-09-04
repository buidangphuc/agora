package service_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"testing"
	"time"

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

func TestAuthService(t *testing.T) {
	repo := repository.NewInMemoryUserRepository()
	svc := service.NewAuthService(repo, testSigner(t), time.Hour)
	ctx := context.Background()

	t.Run("Register success", func(t *testing.T) {
		res, err := svc.Register(ctx, "testuser", "password123", "buyer")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Username != "testuser" {
			t.Errorf("expected testuser, got %s", res.Username)
		}
		if res.Token == "" {
			t.Errorf("expected non-empty token")
		}
	})

	t.Run("Register duplicate username fails", func(t *testing.T) {
		_, err := svc.Register(ctx, "testuser", "password123", "buyer")
		if err == nil {
			t.Errorf("expected duplicate username error")
		}
	})

	t.Run("Register short password fails", func(t *testing.T) {
		_, err := svc.Register(ctx, "newuser", "12", "buyer")
		if err == nil {
			t.Errorf("expected short password error")
		}
	})

	t.Run("Login success", func(t *testing.T) {
		res, err := svc.Login(ctx, "testuser", "password123")
		if err != nil {
			t.Fatalf("unexpected error logging in: %v", err)
		}
		if res.Username != "testuser" {
			t.Errorf("expected testuser, got %s", res.Username)
		}
		if res.Token == "" {
			t.Errorf("expected non-empty token")
		}
	})

	t.Run("Login wrong password fails", func(t *testing.T) {
		_, err := svc.Login(ctx, "testuser", "wrongpassword")
		if err == nil {
			t.Errorf("expected wrong password error")
		}
	})

	t.Run("Login non-existent user fails", func(t *testing.T) {
		_, err := svc.Login(ctx, "nonexistent", "password123")
		if err == nil {
			t.Errorf("expected not found error")
		}
	})

	t.Run("ChangePassword success", func(t *testing.T) {
		reg, err := svc.Register(ctx, "changepass_user", "oldpass123", "buyer")
		if err != nil {
			t.Fatalf("unexpected register error: %v", err)
		}

		err = svc.ChangePassword(ctx, reg.UserID, "oldpass123", "newpass456")
		if err != nil {
			t.Fatalf("unexpected change password error: %v", err)
		}

		// Old password fails
		_, err = svc.Login(ctx, "changepass_user", "oldpass123")
		if err == nil {
			t.Errorf("expected login with old password to fail")
		}

		// New password succeeds
		_, err = svc.Login(ctx, "changepass_user", "newpass456")
		if err != nil {
			t.Fatalf("unexpected login with new password error: %v", err)
		}
	})

	t.Run("ChangePassword wrong old password fails", func(t *testing.T) {
		reg, err := svc.Register(ctx, "changepass_user2", "oldpass123", "buyer")
		if err != nil {
			t.Fatalf("unexpected register error: %v", err)
		}

		err = svc.ChangePassword(ctx, reg.UserID, "wrongoldpass", "newpass456")
		if err == nil {
			t.Errorf("expected error with wrong old password")
		}
	})

	t.Run("ChangePassword short new password fails", func(t *testing.T) {
		reg, err := svc.Register(ctx, "changepass_user3", "oldpass123", "buyer")
		if err != nil {
			t.Fatalf("unexpected register error: %v", err)
		}

		err = svc.ChangePassword(ctx, reg.UserID, "oldpass123", "12")
		if err == nil {
			t.Errorf("expected error with short new password")
		}
	})

	t.Run("ChangePassword non-existent user fails", func(t *testing.T) {
		err := svc.ChangePassword(ctx, "non-existent-user-id", "oldpass123", "newpass456")
		if err == nil {
			t.Errorf("expected error for non-existent user")
		}
	})

	t.Run("RequestPasswordReset success", func(t *testing.T) {
		_, err := svc.Register(ctx, "reset_user", "originalpass123", "buyer")
		if err != nil {
			t.Fatalf("unexpected register error: %v", err)
		}

		token, expiresAt, err := svc.RequestPasswordReset(ctx, "reset_user")
		if err != nil {
			t.Fatalf("unexpected request password reset error: %v", err)
		}
		if token == "" {
			t.Errorf("expected non-empty reset token")
		}
		if !expiresAt.After(time.Now()) {
			t.Errorf("expected expiresAt to be in the future")
		}
	})

	t.Run("RequestPasswordReset non-existent user fails", func(t *testing.T) {
		_, _, err := svc.RequestPasswordReset(ctx, "non_existent_reset_user")
		if err == nil {
			t.Errorf("expected error requesting reset for non-existent user")
		}
	})

	t.Run("ResetPassword success and prevents reuse", func(t *testing.T) {
		_, err := svc.Register(ctx, "reset_user2", "originalpass123", "buyer")
		if err != nil {
			t.Fatalf("unexpected register error: %v", err)
		}

		token, _, err := svc.RequestPasswordReset(ctx, "reset_user2")
		if err != nil {
			t.Fatalf("unexpected request password reset error: %v", err)
		}

		err = svc.ResetPassword(ctx, token, "brandnewpass789")
		if err != nil {
			t.Fatalf("unexpected reset password error: %v", err)
		}

		// Login with new password succeeds
		_, err = svc.Login(ctx, "reset_user2", "brandnewpass789")
		if err != nil {
			t.Fatalf("unexpected login with reset password error: %v", err)
		}

		// Login with old password fails
		_, err = svc.Login(ctx, "reset_user2", "originalpass123")
		if err == nil {
			t.Errorf("expected login with old password to fail")
		}

		// Reusing the same reset token fails
		err = svc.ResetPassword(ctx, token, "anotherpass999")
		if err == nil {
			t.Errorf("expected reset with already used token to fail")
		}
	})

	t.Run("ResetPassword invalid token fails", func(t *testing.T) {
		err := svc.ResetPassword(ctx, "invalid-random-token", "newpass12345")
		if err == nil {
			t.Errorf("expected invalid token error")
		}
	})

	t.Run("ResetPassword short new password fails", func(t *testing.T) {
		token, _, _ := svc.RequestPasswordReset(ctx, "reset_user")
		err := svc.ResetPassword(ctx, token, "12")
		if err == nil {
			t.Errorf("expected short password error")
		}
	})
}
