package handler

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	engagementv1 "github.com/buidangphuc/team-engagement/generated/platform/engagement/v1"
	"github.com/buidangphuc/team-engagement/internal/interceptor"
	"github.com/buidangphuc/team-engagement/internal/service"
	"github.com/buidangphuc/team-engagement/internal/store"
)

// MarkReviewHelpful records the caller's "helpful" vote (idempotent per user).
func (h *EngagementHandler) MarkReviewHelpful(
	ctx context.Context, req *engagementv1.MarkReviewHelpfulRequest,
) (*engagementv1.MarkReviewHelpfulResponse, error) {
	if err := interceptor.RequireScopes(ctx, "engagement:write"); err != nil {
		return nil, err
	}
	if req.GetReviewId() == "" {
		return nil, status.Error(codes.InvalidArgument, "review_id is required")
	}
	if h.reviewSvc == nil {
		return nil, status.Error(codes.Unimplemented, "reviews service unavailable")
	}

	count, err := h.reviewSvc.MarkHelpful(ctx, req.GetReviewId(), userID(ctx))
	if err != nil {
		if errors.Is(err, store.ErrReviewNotFound) {
			return nil, status.Error(codes.NotFound, "review not found")
		}
		return nil, status.Errorf(codes.Internal, "mark helpful: %v", err)
	}
	return &engagementv1.MarkReviewHelpfulResponse{HelpfulCount: count}, nil
}

// GetShopRatingSummary aggregates ratings across a seller's listings.
func (h *EngagementHandler) GetShopRatingSummary(
	ctx context.Context, req *engagementv1.GetShopRatingSummaryRequest,
) (*engagementv1.GetShopRatingSummaryResponse, error) {
	if req.GetSellerId() == "" {
		return nil, status.Error(codes.InvalidArgument, "seller_id is required")
	}
	if h.reviewSvc == nil {
		return &engagementv1.GetShopRatingSummaryResponse{SellerId: req.GetSellerId()}, nil
	}

	summary, err := h.reviewSvc.GetShopRatingSummary(ctx, req.GetSellerId())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get shop rating summary: %v", err)
	}

	return &engagementv1.GetShopRatingSummaryResponse{
		SellerId:      summary.SellerID,
		AverageRating: summary.AverageRating,
		ReviewCount:   summary.ReviewCount,
		Breakdown: &engagementv1.RatingBreakdown{
			Star_1: summary.Breakdown.Star1,
			Star_2: summary.Breakdown.Star2,
			Star_3: summary.Breakdown.Star3,
			Star_4: summary.Breakdown.Star4,
			Star_5: summary.Breakdown.Star5,
		},
	}, nil
}

// HideReview hides a review (admin only). The actor is the verified principal.
func (h *EngagementHandler) HideReview(
	ctx context.Context, req *engagementv1.HideReviewRequest,
) (*engagementv1.HideReviewResponse, error) {
	if err := interceptor.RequireAdmin(ctx); err != nil {
		return nil, err
	}
	if h.reviewSvc == nil {
		return nil, status.Error(codes.Unimplemented, "reviews service unavailable")
	}
	err := h.reviewSvc.HideReview(ctx, userID(ctx), req.GetReviewId(), req.GetReason())
	switch {
	case err == nil:
		return &engagementv1.HideReviewResponse{}, nil
	case errors.Is(err, service.ErrEmptyReviewID), errors.Is(err, service.ErrEmptyReason), errors.Is(err, service.ErrReasonTooLong):
		return nil, status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, store.ErrReviewNotFound):
		return nil, status.Error(codes.NotFound, "review not found")
	case errors.Is(err, service.ErrAuditUnavailable):
		return nil, status.Error(codes.Unavailable, "audit unavailable")
	}
	return nil, status.Error(codes.Internal, "hide review failed")
}
