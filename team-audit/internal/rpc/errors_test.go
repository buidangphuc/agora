package rpc_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	auditv1 "github.com/buidangphuc/team-audit/generated/platform/audit/v1"
)

// Scenario: A cancelled request returns Canceled
func TestCanceledRequestMapsToCanceled(t *testing.T) {
	h, repo := newHandler(t)
	repo.queryErr = context.Canceled
	_, err := h.QueryAuditLog(asAdmin(), &auditv1.QueryAuditLogRequest{})
	if code(err) != codes.Canceled {
		t.Fatalf("got %v, want Canceled", code(err))
	}
}

// Scenario: A timed-out request returns DeadlineExceeded
func TestDeadlineExceededMapsToDeadlineExceeded(t *testing.T) {
	h, repo := newHandler(t)
	repo.appendErr = context.DeadlineExceeded
	_, err := h.WriteAuditEvent(asService("svc-order"), &auditv1.WriteAuditEventRequest{Action: "x"})
	if code(err) != codes.DeadlineExceeded {
		t.Fatalf("got %v, want DeadlineExceeded", code(err))
	}
}

// Scenario: A duplicate primary key returns AlreadyExists
func TestUniqueViolationMapsToAlreadyExists(t *testing.T) {
	h, repo := newHandler(t)
	repo.appendErr = &pgconn.PgError{Code: "23505", ConstraintName: "audit_events_pkey", Message: "duplicate key value violates unique constraint"}
	_, err := h.WriteAuditEvent(asService("svc-order"), &auditv1.WriteAuditEventRequest{Action: "x"})
	if code(err) != codes.AlreadyExists {
		t.Fatalf("got %v, want AlreadyExists", code(err))
	}
	if msg := status.Convert(err).Message(); msg != "already exists" {
		t.Fatalf("message = %q, want %q", msg, "already exists")
	}
}

// Scenario: A panicking repository does not kill the process
func TestPanicIsRecoveredAsInternal(t *testing.T) {
	h, repo := newHandler(t)
	repo.queryPanic = true
	c := dial(t, h, quiet())
	_, err := c.QueryAuditLog(wireAdmin(), &auditv1.QueryAuditLogRequest{})
	if code(err) != codes.Internal {
		t.Fatalf("first call: got %v, want Internal", code(err))
	}
	if msg := status.Convert(err).Message(); strings.Contains(msg, "boom") {
		t.Fatalf("panic text leaked: %q", msg)
	}
	if _, err := c.QueryAuditLog(wireAdmin(), &auditv1.QueryAuditLogRequest{}); err != nil {
		t.Fatalf("server stopped serving after panic: %v", err)
	}
}

// Scenario: A query does not log its filter values
func TestQueryFiltersAreNotLogged(t *testing.T) {
	var buf safeBuf
	logger := capturedLogger(&buf)
	h, repo := newHandler(t)
	h = newHandlerOn(t, repo, logger)
	repo.queryErr = errors.New("db down")
	c := dial(t, h, logger)
	action, target := "secret-action", "secret-target"
	_, err := c.QueryAuditLog(wireAdmin(), &auditv1.QueryAuditLogRequest{Action: &action, TargetId: &target, ActorId: "secret-actor"})
	if code(err) != codes.Internal {
		t.Fatalf("got %v, want Internal", code(err))
	}
	out := buf.String()
	if !strings.Contains(out, "db down") {
		t.Fatalf("expected the failure to be logged, log = %q", out)
	}
	for _, s := range []string{"secret-action", "secret-target", "secret-actor"} {
		if strings.Contains(out, s) {
			t.Fatalf("filter value %q leaked into logs: %s", s, out)
		}
	}
}

// Scenario: A refused query logs no filter values
func TestRefusedQueryLogsNoFilterValues(t *testing.T) {
	var buf safeBuf
	logger := capturedLogger(&buf)
	_, repo := newHandler(t)
	h := newHandlerOn(t, repo, logger)
	c := dial(t, h, logger)
	action, target := "secret-action", "secret-target"
	_, err := c.QueryAuditLog(wireUser(), &auditv1.QueryAuditLogRequest{Action: &action, TargetId: &target, ActorId: "secret-actor"})
	if code(err) != codes.PermissionDenied {
		t.Fatalf("got %v, want PermissionDenied", code(err))
	}
	if repo.queries != 0 {
		t.Fatalf("repository must not be called")
	}
	out := buf.String()
	if out == "" {
		t.Fatalf("expected the refused call to be logged")
	}
	for _, s := range []string{"secret-action", "secret-target", "secret-actor"} {
		if strings.Contains(out, s) {
			t.Fatalf("filter value %q leaked into logs: %s", s, out)
		}
	}
}
