package handler_test

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	identityv1 "github.com/buidangphuc/team-identity/generated/platform/identity/v1"
	"github.com/buidangphuc/team-identity/internal/handler"
	"github.com/buidangphuc/team-identity/internal/repository"
	"github.com/buidangphuc/team-identity/internal/service"
)

func TestRequestPasswordReset_LogsIssuanceWithFingerprintNotToken(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	svc := service.NewAuthService(repository.NewInMemoryUserRepository(), testSigner(t), time.Hour)
	h := handler.NewAuthHandler(svc).WithLogger(logger).WithExposeResetToken(true)
	ctx := context.Background()
	reg, err := h.Register(ctx, &identityv1.RegisterRequest{Username: "audit_user", Password: "password123", Role: "buyer"})
	if err != nil {
		t.Fatal(err)
	}
	res, err := h.RequestPasswordReset(ctx, &identityv1.RequestPasswordResetRequest{Username: "audit_user"})
	if err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	uid := reg.GetResult().GetPrincipal().GetId()
	for _, want := range []string{"password_reset.issued", "user_id=" + uid, "expires_at=", "token_fingerprint=" + service.TokenFingerprint(res.ResetToken)} {
		if !strings.Contains(out, want) {
			t.Errorf("log missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, res.ResetToken) {
		t.Errorf("raw reset token leaked into log:\n%s", out)
	}
	// A failed request (unknown user) issues nothing and logs nothing.
	buf.Reset()
	if _, err := h.RequestPasswordReset(ctx, &identityv1.RequestPasswordResetRequest{Username: "nobody"}); err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(buf.String(), "password_reset.issued") {
		t.Errorf("failed request must not log issuance: %s", buf.String())
	}
}
