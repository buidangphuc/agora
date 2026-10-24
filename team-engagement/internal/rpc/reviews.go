package rpc

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	engagementv1 "github.com/buidangphuc/team-engagement/generated/platform/engagement/v1"
)

func (h *Handler) MarkReviewHelpful(
	ctx context.Context, req *engagementv1.MarkReviewHelpfulRequest,
) (*engagementv1.MarkReviewHelpfulResponse, error) {
	if err := requireScopes(ctx, "engagement:write"); err != nil {
		return nil, err
	}
	if req.GetReviewId() == "" {
		return nil, status.Error(codes.InvalidArgument, "review_id is required")
	}

	count, err := h.reviewSvc.MarkHelpful(ctx, req.GetReviewId(), userID(ctx))
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	return &engagementv1.MarkReviewHelpfulResponse{HelpfulCount: count}, nil
}

func (h *Handler) GetShopRatingSummary(
	ctx context.Context, req *engagementv1.GetShopRatingSummaryRequest,
) (*engagementv1.GetShopRatingSummaryResponse, error) {
	if req.GetSellerId() == "" {
		return nil, status.Error(codes.InvalidArgument, "seller_id is required")
	}

	summary, err := h.reviewSvc.GetShopRatingSummary(ctx, req.GetSellerId())
	if err != nil {
		return nil, h.fail(ctx, err)
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

func (h *Handler) HideReview(
	ctx context.Context, req *engagementv1.HideReviewRequest,
) (*engagementv1.HideReviewResponse, error) {
	if err := requireScopes(ctx, "admin"); err != nil {
		return nil, err
	}
	err := h.reviewSvc.HideReview(ctx, userID(ctx), req.GetReviewId(), req.GetReason())
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	return &engagementv1.HideReviewResponse{}, nil
}
