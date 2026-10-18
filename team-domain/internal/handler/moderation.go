package handler

import (
	"context"
	"errors"
	"unicode/utf8"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	commonv1 "github.com/buidangphuc/team-domain/generated/platform/common/v1"
	listingv1 "github.com/buidangphuc/team-domain/generated/platform/listing/v1"
	"github.com/buidangphuc/team-domain/internal/repository"
	"github.com/buidangphuc/team-domain/internal/service"
)

const maxModerationNoteChars = 500

var moderationReasonCodes = map[string]bool{
	"prohibited_item": true, "counterfeit": true, "misleading": true,
	"inappropriate": true, "duplicate": true, "seller_enforcement": true, "other": true,
}

func requireAdmin(ctx context.Context) error {
	id, admin := principalOwner(ctx)
	if id == "" {
		return status.Error(codes.Unauthenticated, "unauthenticated")
	}
	if !admin {
		return status.Error(codes.PermissionDenied, "permission denied")
	}
	return nil
}

func decisionFromWire(d listingv1.ModerationDecision) (service.ModerationDecision, bool) {
	switch d {
	case listingv1.ModerationDecision_MODERATION_DECISION_APPROVE:
		return service.DecisionApprove, true
	case listingv1.ModerationDecision_MODERATION_DECISION_REJECT:
		return service.DecisionReject, true
	case listingv1.ModerationDecision_MODERATION_DECISION_BAN:
		return service.DecisionBan, true
	}
	return "", false
}

// ModerateListing applies an admin decision (admin only, audited first).
func (h *ListingHandler) ModerateListing(ctx context.Context, req *listingv1.ModerateListingRequest) (*listingv1.ModerateListingResponse, error) {
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	dec, ok := decisionFromWire(req.GetDecision())
	switch {
	case req.GetListingId() == "":
		return nil, status.Error(codes.InvalidArgument, "listing_id is required")
	case !ok:
		return nil, status.Error(codes.InvalidArgument, "decision is required")
	case dec != service.DecisionApprove && req.GetReasonCode() == "":
		return nil, status.Error(codes.InvalidArgument, "reason_code is required")
	case req.GetReasonCode() != "" && !moderationReasonCodes[req.GetReasonCode()]:
		return nil, status.Error(codes.InvalidArgument, "unknown reason_code")
	case utf8.RuneCountInString(req.GetNote()) > maxModerationNoteChars:
		return nil, status.Error(codes.InvalidArgument, "note exceeds 500 characters")
	}
	actor, _ := principalOwner(ctx)
	l, err := h.svc.ModerateListing(ctx, service.ModerateInput{
		ListingID: req.GetListingId(), Decision: dec, ReasonCode: req.GetReasonCode(), Note: req.GetNote(),
	}, actor, h.enqueueListingChanged(ctx, listingv1.ChangeType_CHANGE_TYPE_UPDATED))
	if err != nil {
		switch {
		case errors.Is(err, repository.ErrNotFound):
			return nil, status.Error(codes.NotFound, "not_found")
		case errors.Is(err, service.ErrAuditUnavailable):
			return nil, status.Error(codes.Unavailable, "audit unavailable")
		case errors.Is(err, service.ErrTransitionNotAllowed), errors.Is(err, repository.ErrStatusChanged):
			return nil, status.Error(codes.FailedPrecondition, "listing status does not allow this decision")
		default:
			return nil, internalErr("ModerateListing", err)
		}
	}
	return &listingv1.ModerateListingResponse{Listing: toWireFor(ctx, l)}, nil
}

// ListModerationQueue lists pending_review listings, oldest first (admin only, not audited).
func (h *ListingHandler) ListModerationQueue(ctx context.Context, req *listingv1.ListModerationQueueRequest) (*listingv1.ListModerationQueueResponse, error) {
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	var cursor string
	var size int32
	if p := req.GetPage(); p != nil {
		cursor, size = p.GetCursor(), p.GetPageSize()
	}
	page, err := h.svc.ListModerationQueue(ctx, cursor, size)
	if err != nil {
		if errors.Is(err, repository.ErrInvalidCursor) {
			return nil, status.Error(codes.InvalidArgument, "invalid cursor")
		}
		return nil, internalErr("ListModerationQueue", err)
	}
	items := make([]*listingv1.ModerationQueueItem, 0, len(page.Items))
	for _, l := range page.Items {
		it := &listingv1.ModerationQueueItem{Listing: toWireFor(ctx, l)}
		if !l.SubmittedAt.IsZero() {
			it.SubmittedAt = timestamppb.New(l.SubmittedAt)
		}
		items = append(items, it)
	}
	return &listingv1.ListModerationQueueResponse{
		Items: items,
		Page:  &commonv1.PageResponse{NextCursor: page.NextCursor, Total: page.Total},
	}, nil
}
