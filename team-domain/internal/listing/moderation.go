package listing

import (
	"context"
	"errors"
	"time"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auditx"
	"github.com/buidangphuc/team-domain/internal/store"
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
	ModerateTx(ctx context.Context, id, fromStatus string, loadedUpdatedAt time.Time, d store.ModerationUpdate, ev store.EnqueueFn) (store.Listing, error)
}

// moderationQueue is the pending_review read (PostgresListingRepository provides it).
type moderationQueue interface {
	ListModerationQueue(ctx context.Context, cursor string, pageSize int32) (store.Page, error)
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
func (s *ListingService) ModerateListing(ctx context.Context, in ModerateInput, actorID string, enqueue store.EnqueueFn) (store.Listing, error) {
	existing, err := s.repo.Get(ctx, in.ListingID)
	if err != nil {
		return store.Listing{}, err
	}
	to, ok := moderationTarget(existing.Status, in.Decision)
	if !ok {
		return store.Listing{}, ErrTransitionNotAllowed
	}
	mtx, ok := s.tx.(moderationTx)
	if !ok {
		return store.Listing{}, errors.New("moderation writer not configured")
	}
	ev := auditx.Event{
		ActorID: actorID, Action: "listing.moderate", TargetType: "listing", TargetID: existing.ID,
		Metadata: map[string]string{
			"seller_id": existing.SellerID, "decision": string(in.Decision), "reason_code": in.ReasonCode, "note": in.Note,
			"from_status": existing.Status, "to_status": to,
		},
	}
	var moderated store.Listing
	err = s.audit.Do(ctx, ev, func(ctx context.Context) error {
		var werr error
		moderated, werr = mtx.ModerateTx(ctx, existing.ID, existing.Status, existing.UpdatedAt, store.ModerationUpdate{
			ToStatus:   to,
			ReasonCode: in.ReasonCode,
			Note:       in.Note,
			DecidedAt:  time.Now(),
		}, enqueue)
		return werr
	})
	return moderated, err
}

// ListModerationQueue returns pending_review listings, oldest submission first.
func (s *ListingService) ListModerationQueue(ctx context.Context, cursor string, pageSize int32) (store.Page, error) {
	q, ok := s.repo.(moderationQueue)
	if !ok {
		return store.Page{}, errors.New("moderation queue not supported by repository")
	}
	return q.ListModerationQueue(ctx, cursor, pageSize)
}
