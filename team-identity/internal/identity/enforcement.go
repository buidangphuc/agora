package identity

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auditx"
	commonv1 "github.com/buidangphuc/team-identity/generated/platform/common/v1"
	identityv1 "github.com/buidangphuc/team-identity/generated/platform/identity/v1"
	"github.com/buidangphuc/team-identity/internal/authz"
	"github.com/buidangphuc/team-identity/internal/events"
	"github.com/buidangphuc/team-identity/internal/store"
)

// ErrStatusPrecondition: self target or admin target.
var ErrStatusPrecondition = errors.New("status precondition failed")

// statusWriter is the optional repository surface for enforcement.
type statusWriter interface {
	SetStatus(ctx context.Context, userID, status, reason string, until, now time.Time, revoke store.EnqueueFn, changed store.StatusEnqueueFn) (store.StatusResult, error)
	LiftExpiredSuspension(ctx context.Context, userID string, now time.Time, changed store.StatusEnqueueFn) (bool, error)
}

func protoStatus(status string) identityv1.UserStatus {
	switch status {
	case store.StatusActive:
		return identityv1.UserStatus_USER_STATUS_ACTIVE
	case store.StatusRestricted:
		return identityv1.UserStatus_USER_STATUS_RESTRICTED
	case store.StatusSuspended:
		return identityv1.UserStatus_USER_STATUS_SUSPENDED
	case store.StatusBanned:
		return identityv1.UserStatus_USER_STATUS_BANNED
	}
	return identityv1.UserStatus_USER_STATUS_UNSPECIFIED
}

// statusEvent builds the UserStatusChanged outbox row inside the write transaction.
func (s *AuthService) statusEvent(ctx context.Context, reasonOverride string) store.StatusEnqueueFn {
	requestID := requestIDFromCtx(ctx)
	return func(c store.StatusChange) (store.OutboxRow, error) {
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
			return store.OutboxRow{}, err
		}
		return store.OutboxRow{
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
func (s *AuthService) SetUserStatus(ctx context.Context, actorID, userID, status, reasonCode, note string, until *time.Time) (store.StatusResult, error) {
	now := s.clock()
	userID = strings.TrimSpace(userID)
	reasonCode = strings.TrimSpace(reasonCode)
	switch status {
	case store.StatusActive, store.StatusRestricted, store.StatusBanned:
		if until != nil {
			return store.StatusResult{}, fmt.Errorf("%w: until only applies to suspended", ErrInvalidInput)
		}
	case store.StatusSuspended:
		if until == nil || !until.After(now) {
			return store.StatusResult{}, fmt.Errorf("%w: suspended needs a future until", ErrInvalidInput)
		}
	default:
		return store.StatusResult{}, fmt.Errorf("%w: unknown status", ErrInvalidInput)
	}
	if userID == "" || reasonCode == "" {
		return store.StatusResult{}, fmt.Errorf("%w: user id and reason are required", ErrInvalidInput)
	}
	sw, ok := s.repo.(statusWriter)
	if !ok {
		return store.StatusResult{}, errors.New("user repository does not support status changes")
	}
	target, err := s.repo.GetByID(ctx, userID)
	if err != nil {
		return store.StatusResult{}, err
	}
	if actorID == target.ID {
		return store.StatusResult{}, fmt.Errorf("%w: cannot change own status", ErrStatusPrecondition)
	}
	for _, r := range target.Roles {
		if r == authz.RoleAdmin {
			return store.StatusResult{}, fmt.Errorf("%w: cannot change an admin", ErrStatusPrecondition)
		}
	}
	var untilT time.Time
	if until != nil {
		untilT = *until
	}
	meta := map[string]string{"status": status, "reason_code": reasonCode}
	if note != "" {
		meta["note"] = note
	}
	if until != nil {
		meta["until"] = until.UTC().Format(time.RFC3339)
	}
	var res store.StatusResult
	err = s.gate.Do(ctx, auditx.Event{ActorID: actorID, Action: "user.set_status", TargetType: "user", TargetID: userID, Metadata: meta},
		func(ctx context.Context) error {
			var serr error
			res, serr = sw.SetStatus(ctx, userID, status, reasonCode, untilT, now, s.revokedEvent(ctx, now), s.statusEvent(ctx, ""))
			return serr
		})
	if err != nil {
		return store.StatusResult{}, err
	}
	return res, nil
}

// liftIfExpired returns the user as it should be gated: an expired suspension is
// lifted (no audit); a failed lift is an error (fail closed).
func (s *AuthService) liftIfExpired(ctx context.Context, u store.User) (store.User, error) {
	now := s.clock()
	if u.Status != store.StatusSuspended || u.StatusUntil.IsZero() || u.StatusUntil.After(now) {
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
