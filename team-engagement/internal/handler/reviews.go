package handler

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	engagementv1 "github.com/buidangphuc/team-engagement/generated/platform/engagement/v1"
	"github.com/buidangphuc/team-engagement/internal/interceptor"
	"github.com/buidangphuc/team-engagement/internal/repository"
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
		if errors.Is(err, repository.ErrReviewNotFound) {
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
