// Package audit holds team-audit's domain logic. Service owns the append-only
// audit trail: it validates input and delegates persistence to the store. The
// transport layer resolves the caller's identity and passes it in; the service
// never guesses. Events are immutable — there is intentionally no update/delete.
package audit

import (
	"context"
	"errors"

	auditv1 "github.com/buidangphuc/team-audit/generated/platform/audit/v1"
	"github.com/buidangphuc/team-audit/internal/store"
)

// Validation error surfaced to the transport layer, which maps it to a gRPC code.
var (
	// ErrEmptyAction rejects a write with no action verb — an audit event that
	// does not name what happened records nothing useful.
	ErrEmptyAction = errors.New("action is required")
)

// MaxPageSize caps a single QueryAuditLog page so a caller cannot ask for an
// unbounded scan of the trail.
const MaxPageSize = 100

// DefaultPageSize is used when a caller omits page_size (0).
const DefaultPageSize = 20

// Service is the audit use-case boundary.
type Service struct {
	repo store.AuditRepository
}

// NewService returns a Service that persists events through repo.
func NewService(repo store.AuditRepository) *Service {
	return &Service{repo: repo}
}

// Write appends an immutable audit event. actor_id may be empty (anonymous /
// system actor); action is required. Returns the stored event (with server id +
// created_at) so a caller that wants the receipt has it, while the RPC itself is
// fire-and-forget (its response carries no payload).
func (s *Service) Write(ctx context.Context, actorID, action, targetType, targetID string, metadata map[string]string) (*auditv1.AuditEvent, error) {
	if action == "" {
		return nil, ErrEmptyAction
	}
	return s.repo.Append(ctx, &auditv1.AuditEvent{
		ActorId:    actorID,
		Action:     action,
		TargetType: targetType,
		TargetId:   targetID,
		Metadata:   metadata,
	})
}

// Query returns audit events matching the filter, newest first, and the total
// count of matches (before paging). pageSize is clamped to [1, MaxPageSize].
func (s *Service) Query(ctx context.Context, actorID, targetType string, pageSize, offset int) ([]*auditv1.AuditEvent, int64, error) {
	if pageSize <= 0 {
		pageSize = DefaultPageSize
	}
	if pageSize > MaxPageSize {
		pageSize = MaxPageSize
	}
	if offset < 0 {
		offset = 0
	}
	return s.repo.Query(ctx, store.QueryFilter{
		ActorID:    actorID,
		TargetType: targetType,
		Limit:      pageSize,
		Offset:     offset,
	})
}
