package service

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/buidangphuc/team-domain/internal/repository"
)

// ModerationDecision is an admin decision on a listing.
type ModerationDecision string

const (
	DecisionApprove ModerationDecision = "approve"
	DecisionReject  ModerationDecision = "reject"
	DecisionBan     ModerationDecision = "ban"
)

// ModerateInput is one admin decision.
type ModerateInput struct {
	ListingID  string
	Decision   ModerationDecision
	ReasonCode string
	Note       string
}

// ErrTransitionNotAllowed: the stored status does not allow this decision. The
// handler maps it to codes.FailedPrecondition.
var ErrTransitionNotAllowed = errors.New("moderation transition not allowed")

// moderationTx is the conditional decision write (PgTxWriter provides it).
type moderationTx interface {
	ModerateTx(ctx context.Context, id, fromStatus string, loadedUpdatedAt time.Time, d repository.ModerationUpdate, ev repository.EnqueueFn) (repository.Listing, error)
}

// moderationQueue is the pending_review read (PostgresListingRepository provides it).
type moderationQueue interface {
	ListModerationQueue(ctx context.Context, cursor string, pageSize int32) (repository.Page, error)
}

// moderationTarget returns the status a decision leads to from the stored
// status, or false when the transition is refused.
func moderationTarget(from string, d ModerationDecision) (string, bool) {
	switch from {
	case "pending_review":
		switch d {
		case DecisionApprove:
			return "published", true
		case DecisionReject:
			return "rejected", true
		case DecisionBan:
			return "banned", true
		}
	case "published":
		switch d {
		case DecisionReject:
			return "rejected", true
		case DecisionBan:
			return "banned", true
		}
	}
	return "", false
}

// ModerateListing applies an admin decision: load, check the transition, write
// the audit event, then apply the decision conditionally on the loaded version
// (so a racing seller edit is refused) together with its outbox event.
func (s *ListingService) ModerateListing(ctx context.Context, in ModerateInput, actorID string, enqueue repository.EnqueueFn) (repository.Listing, error) {
	existing, err := s.repo.Get(ctx, in.ListingID)
	if err != nil {
		return repository.Listing{}, err
	}
	to, ok := moderationTarget(existing.Status, in.Decision)
	if !ok {
		return repository.Listing{}, ErrTransitionNotAllowed
	}
	mtx, ok := s.tx.(moderationTx)
	if !ok {
		return repository.Listing{}, errors.New("moderation writer not configured")
	}
	if s.audit == nil {
		return repository.Listing{}, ErrAuditUnavailable
	}
	if err := s.audit.WriteListingModerate(ctx, actorID, existing.ID, existing.SellerID, string(in.Decision), in.ReasonCode, in.Note, existing.Status, to); err != nil {
		slog.WarnContext(ctx, "listing moderation audit failed", slog.String("listing_id", existing.ID), slog.Any("err", err))
		return repository.Listing{}, ErrAuditUnavailable
	}
	return mtx.ModerateTx(ctx, existing.ID, existing.Status, existing.UpdatedAt, repository.ModerationUpdate{
		ToStatus:   to,
		ReasonCode: in.ReasonCode,
		Note:       in.Note,
		DecidedAt:  time.Now(),
	}, enqueue)
}

// ListModerationQueue returns pending_review listings, oldest submission first.
func (s *ListingService) ListModerationQueue(ctx context.Context, cursor string, pageSize int32) (repository.Page, error) {
	q, ok := s.repo.(moderationQueue)
	if !ok {
		return repository.Page{}, errors.New("moderation queue not supported by repository")
	}
	return q.ListModerationQueue(ctx, cursor, pageSize)
}
