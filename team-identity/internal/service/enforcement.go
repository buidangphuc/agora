package service

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/google/uuid"

	commonv1 "github.com/buidangphuc/team-identity/generated/platform/common/v1"
	identityv1 "github.com/buidangphuc/team-identity/generated/platform/identity/v1"
	"github.com/buidangphuc/team-identity/internal/authz"
	"github.com/buidangphuc/team-identity/internal/events"
	"github.com/buidangphuc/team-identity/internal/repository"
)

var (
	// ErrAuditUnavailable: audit not configured, errored or timed out; the change is refused.
	ErrAuditUnavailable = errors.New("audit unavailable")
	// ErrStatusPrecondition: self target or admin target.
	ErrStatusPrecondition = errors.New("status precondition failed")
)

// AuditWriter is satisfied by *upstream.AuditClient (including a typed-nil one).
type AuditWriter interface {
	WriteUserSetStatus(ctx context.Context, actorID, userID, status, reasonCode, note string, until *time.Time) error
}

// statusWriter is the optional repository surface for enforcement.
type statusWriter interface {
	SetStatus(ctx context.Context, userID, status, reason string, until, now time.Time, revoke repository.EnqueueFn, changed repository.StatusEnqueueFn) (repository.StatusResult, error)
	LiftExpiredSuspension(ctx context.Context, userID string, now time.Time, changed repository.StatusEnqueueFn) (bool, error)
}

// WithAudit sets the audit writer used by SetUserStatus.
func (s *AuthService) WithAudit(w AuditWriter) *AuthService {
	s.audit = w
	return s
}

func (s *AuthService) auditConfigured() bool {
	if s.audit == nil {
		return false
	}
	v := reflect.ValueOf(s.audit)
	return !(v.Kind() == reflect.Ptr && v.IsNil())
}

func protoStatus(status string) identityv1.UserStatus {
	switch status {
	case repository.StatusActive:
		return identityv1.UserStatus_USER_STATUS_ACTIVE
	case repository.StatusRestricted:
		return identityv1.UserStatus_USER_STATUS_RESTRICTED
	case repository.StatusSuspended:
		return identityv1.UserStatus_USER_STATUS_SUSPENDED
	case repository.StatusBanned:
		return identityv1.UserStatus_USER_STATUS_BANNED
	}
	return identityv1.UserStatus_USER_STATUS_UNSPECIFIED
}

// statusEvent builds the UserStatusChanged outbox row inside the write transaction.
func (s *AuthService) statusEvent(ctx context.Context, reasonOverride string) repository.StatusEnqueueFn {
	requestID := requestIDFromCtx(ctx)
	return func(c repository.StatusChange) (repository.OutboxRow, error) {
		eventID := uuid.NewString()
		reason := c.Reason
		if reasonOverride != "" {
			reason = reasonOverride
		}
		var until *time.Time
		if !c.Until.IsZero() {
			u := c.Until
			until = &u
		}
		principal := &commonv1.Principal{Id: c.UserID, Type: commonv1.PrincipalType_PRINCIPAL_TYPE_USER}
		payload, err := events.BuildUserStatusChangedEnvelope(eventID, c.UserID,
			protoStatus(c.Status), protoStatus(c.PreviousStatus), reason, until, c.ChangedAt, principal, requestID)
		if err != nil {
			return repository.OutboxRow{}, err
		}
		return repository.OutboxRow{
			EventID:       eventID,
			AggregateType: "User",
			AggregateID:   c.UserID,
			EventType:     events.UserStatusChangedEventType,
			Payload:       payload,
			RequestID:     requestID,
		}, nil
	}
}

// SetUserStatus: validate, load, preconditions, audit, transaction.
func (s *AuthService) SetUserStatus(ctx context.Context, actorID, userID, status, reasonCode, note string, until *time.Time) (repository.StatusResult, error) {
	now := s.clock()
	userID = strings.TrimSpace(userID)
	reasonCode = strings.TrimSpace(reasonCode)
	switch status {
	case repository.StatusActive, repository.StatusRestricted, repository.StatusBanned:
		if until != nil {
			return repository.StatusResult{}, fmt.Errorf("%w: until only applies to suspended", ErrInvalidInput)
		}
	case repository.StatusSuspended:
		if until == nil || !until.After(now) {
			return repository.StatusResult{}, fmt.Errorf("%w: suspended needs a future until", ErrInvalidInput)
		}
	default:
		return repository.StatusResult{}, fmt.Errorf("%w: unknown status", ErrInvalidInput)
	}
	if userID == "" || reasonCode == "" {
		return repository.StatusResult{}, fmt.Errorf("%w: user id and reason are required", ErrInvalidInput)
	}
	sw, ok := s.repo.(statusWriter)
	if !ok {
		return repository.StatusResult{}, errors.New("user repository does not support status changes")
	}
	target, err := s.repo.GetByID(ctx, userID)
	if err != nil {
		return repository.StatusResult{}, err
	}
	if actorID == target.ID {
		return repository.StatusResult{}, fmt.Errorf("%w: cannot change own status", ErrStatusPrecondition)
	}
	for _, r := range target.Roles {
		if r == authz.RoleAdmin {
			return repository.StatusResult{}, fmt.Errorf("%w: cannot change an admin", ErrStatusPrecondition)
		}
	}
	if !s.auditConfigured() {
		return repository.StatusResult{}, ErrAuditUnavailable
	}
	if err := s.audit.WriteUserSetStatus(ctx, actorID, userID, status, reasonCode, note, until); err != nil {
		return repository.StatusResult{}, fmt.Errorf("%w: %v", ErrAuditUnavailable, err)
	}
	var untilT time.Time
	if until != nil {
		untilT = *until
	}
	return sw.SetStatus(ctx, userID, status, reasonCode, untilT, now, s.revokedEvent(ctx, now), s.statusEvent(ctx, ""))
}

// liftIfExpired returns the user as it should be gated: an expired suspension is
// lifted (no audit); a failed lift is an error (fail closed).
func (s *AuthService) liftIfExpired(ctx context.Context, u repository.User) (repository.User, error) {
	now := s.clock()
	if u.Status != repository.StatusSuspended || u.StatusUntil.IsZero() || u.StatusUntil.After(now) {
		return u, nil
	}
	sw, ok := s.repo.(statusWriter)
	if !ok {
		return u, errors.New("user repository does not support suspension lift")
	}
	_, err := sw.LiftExpiredSuspension(ctx, u.ID, now, s.statusEvent(ctx, "suspension_expired"))
	if err != nil {
		return u, fmt.Errorf("lift suspension: %w", err)
	}
	// Re-read either way: a miss means a concurrent change (e.g. a ban) won.
	fresh, err := s.repo.GetByID(ctx, u.ID)
	if err != nil {
		return u, err
	}
	return fresh, nil
}
