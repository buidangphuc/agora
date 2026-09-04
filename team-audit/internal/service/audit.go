// Package service holds team-audit's application logic. AuditService owns the
// append-only audit trail: it validates input and delegates persistence to the
// repository. Handlers resolve the caller's identity and pass it in; the service
// never guesses. Events are immutable — there is intentionally no update/delete.
package service

import (
	"context"
	"errors"

	auditv1 "github.com/buidangphuc/team-audit/generated/platform/audit/v1"
	"github.com/buidangphuc/team-audit/internal/repository"
)

// Validation error surfaced to the handler, which maps it to a gRPC code.
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

// AuditService is the audit use-case boundary.
type AuditService struct {
	repo repository.AuditRepository
}

func NewAuditService(repo repository.AuditRepository) *AuditService {
	return &AuditService{repo: repo}
}

// Write appends an immutable audit event. actor_id may be empty (anonymous /
// system actor); action is required. Returns the stored event (with server id +
// created_at) so a caller that wants the receipt has it, while the RPC itself is
// fire-and-forget (its response carries no payload).
func (s *AuditService) Write(ctx context.Context, actorID, action, targetType, targetID string, metadata map[string]string) (*auditv1.AuditEvent, error) {
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
func (s *AuditService) Query(ctx context.Context, actorID, targetType string, pageSize, offset int) ([]*auditv1.AuditEvent, int64, error) {
	if pageSize <= 0 {
		pageSize = DefaultPageSize
	}
	if pageSize > MaxPageSize {
		pageSize = MaxPageSize
	}
	if offset < 0 {
		offset = 0
	}
	return s.repo.Query(ctx, repository.QueryFilter{
		ActorID:    actorID,
		TargetType: targetType,
		Limit:      pageSize,
		Offset:     offset,
	})
}
