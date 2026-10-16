package service

// TEST-STUB (task 6, identity-user-enforcement): compile-only declarations so the
// red tests build. The implementer deletes this file and defines the real symbols
// in auth.go / enforcement.go.

import (
	"context"
	"errors"
	"time"

	"github.com/buidangphuc/team-identity/internal/repository"
)

// ErrAccountLocked: login/refresh refusal for suspended and banned users.
var ErrAccountLocked = errors.New("account locked")

// ErrAuditUnavailable: audit not configured, errored or timed out; the change is refused.
var ErrAuditUnavailable = errors.New("audit unavailable")

// ErrStatusPrecondition: self target or admin target.
var ErrStatusPrecondition = errors.New("status precondition failed")

// AuditWriter is satisfied by *upstream.AuditClient (including a typed-nil one).
type AuditWriter interface {
	WriteUserSetStatus(ctx context.Context, actorID, userID, status, reasonCode, note string, until *time.Time) error
}

// WithAudit sets the audit writer used by SetUserStatus.
func (s *AuthService) WithAudit(w AuditWriter) *AuthService { return s }

// SetUserStatus: validate, load, preconditions, audit, transaction.
func (s *AuthService) SetUserStatus(ctx context.Context, actorID, userID, status, reasonCode, note string, until *time.Time) (repository.StatusResult, error) {
	return repository.StatusResult{}, errors.New("not implemented")
}
