package identity_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auditx"
	"github.com/buidangphuc/team-identity/internal/identity"
	"github.com/buidangphuc/team-identity/internal/store"
)

type nopAudit struct{}

func (nopAudit) WriteAuditEvent(context.Context, auditx.Event) error { return nil }

func newGate(w auditx.Writer) *auditx.Gate {
	return auditx.NewGate(w, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func newSvcAudit(t *testing.T, users store.UserRepository, sessions store.SessionRepository, w auditx.Writer) *identity.AuthService {
	t.Helper()
	svc, err := identity.NewAuthService(users, sessions, testSigner(t), time.Hour, newGate(w))
	if err != nil {
		t.Fatalf("NewAuthService: %v", err)
	}
	return svc
}

func newSvc(t *testing.T, users store.UserRepository, sessions store.SessionRepository) *identity.AuthService {
	t.Helper()
	return newSvcAudit(t, users, sessions, nopAudit{})
}
