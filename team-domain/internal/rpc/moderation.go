package rpc

import (
	"context"
	"unicode/utf8"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	commonv1 "github.com/buidangphuc/team-domain/generated/platform/common/v1"
	listingv1 "github.com/buidangphuc/team-domain/generated/platform/listing/v1"
	"github.com/buidangphuc/team-domain/internal/listing"
)

const maxModerationNoteChars = 500

var moderationReasonCodes = map[string]bool{
	"prohibited_item": true, "counterfeit": true, "misleading": true,
	"inappropriate": true, "duplicate": true, "seller_enforcement": true, "other": true,
}

func decisionFromWire(d listingv1.ModerationDecision) (listing.ModerationDecision, bool) {
	switch d {
	case listingv1.ModerationDecision_MODERATION_DECISION_APPROVE:
		return listing.DecisionApprove, true
	case listingv1.ModerationDecision_MODERATION_DECISION_REJECT:
		return listing.DecisionReject, true
	case listingv1.ModerationDecision_MODERATION_DECISION_BAN:
		return listing.DecisionBan, true
	}
	return "", false
}

// ModerateListing applies an admin decision (admin only, audited first).
func (h *ListingHandler) ModerateListing(ctx context.Context, req *listingv1.ModerateListingRequest) (*listingv1.ModerateListingResponse, error) {
	if _, err := auth.RequireScopes(ctx, "admin"); err != nil {
		return nil, err
	}
	dec, ok := decisionFromWire(req.GetDecision())
	switch {
	case req.GetListingId() == "":
		return nil, status.Error(codes.InvalidArgument, "listing_id is required")
	case !ok:
		return nil, status.Error(codes.InvalidArgument, "decision is required")
	case dec != listing.DecisionApprove && req.GetReasonCode() == "":
		return nil, status.Error(codes.InvalidArgument, "reason_code is required")
	case req.GetReasonCode() != "" && !moderationReasonCodes[req.GetReasonCode()]:
		return nil, status.Error(codes.InvalidArgument, "unknown reason_code")
	case utf8.RuneCountInString(req.GetNote()) > maxModerationNoteChars:
		return nil, status.Error(codes.InvalidArgument, "note exceeds 500 characters")
	}
	actor, _ := principalOwner(ctx)
	l, err := h.svc.ModerateListing(ctx, listing.ModerateInput{
		ListingID: req.GetListingId(), Decision: dec, ReasonCode: req.GetReasonCode(), Note: req.GetNote(),
	}, actor, h.enqueueListingChanged(ctx, listingv1.ChangeType_CHANGE_TYPE_UPDATED))
	if err != nil {
		return nil, fail(ctx, err)
	}
	return &listingv1.ModerateListingResponse{Listing: toWireFor(ctx, l)}, nil
}

// ListModerationQueue lists pending_review listings, oldest first (admin only, not audited).
func (h *ListingHandler) ListModerationQueue(ctx context.Context, req *listingv1.ListModerationQueueRequest) (*listingv1.ListModerationQueueResponse, error) {
	if _, err := auth.RequireScopes(ctx, "admin"); err != nil {
		return nil, err
	}
	var cursor string
	var size int32
	if p := req.GetPage(); p != nil {
		cursor, size = p.GetCursor(), p.GetPageSize()
	}
	page, err := h.svc.ListModerationQueue(ctx, cursor, size)
	if err != nil {
		return nil, fail(ctx, err)
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
