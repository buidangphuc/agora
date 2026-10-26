package rpc

import (
	"context"
	"log/slog"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	promotionv1 "github.com/buidangphuc/team-promotion/generated/platform/promotion/v1"
	"github.com/buidangphuc/team-promotion/internal/sponsored"
)

type SponsoredHandler struct {
	promotionv1.UnimplementedSponsoredServiceServer

	svc    *sponsored.Service
	logger *slog.Logger
}

func NewSponsoredHandler(svc *sponsored.Service, logger *slog.Logger) *SponsoredHandler {
	return &SponsoredHandler{svc: svc, logger: orDefault(logger)}
}

func (h *SponsoredHandler) CreateAdCampaign(ctx context.Context, req *promotionv1.CreateAdCampaignRequest) (*promotionv1.CreateAdCampaignResponse, error) {
	principal, err := auth.RequireScopes(ctx, scopeListingWrite)
	if err != nil {
		return nil, err
	}
	if req.GetListingId() == "" {
		return nil, status.Error(codes.InvalidArgument, "listing_id is required")
	}
	campaign, err := h.svc.CreateAdCampaign(ctx, sponsored.CreateAdCampaignParams{
		SellerID:  principal.ID,
		ListingID: req.GetListingId(),
		Budget:    req.GetBudget(),
		Bid:       req.GetBid(),
		IsAdmin:   principal.HasScope(scopeAdmin),
	})
	if err != nil {
		return nil, fail(ctx, h.logger, err)
	}
	return &promotionv1.CreateAdCampaignResponse{Campaign: adCampaignToProto(campaign)}, nil
}

func (h *SponsoredHandler) ListSponsoredSlots(ctx context.Context, req *promotionv1.ListSponsoredSlotsRequest) (*promotionv1.ListSponsoredSlotsResponse, error) {
	ids, err := h.svc.ListSponsoredSlots(ctx, req.GetContextStr())
	if err != nil {
		return nil, fail(ctx, h.logger, err)
	}
	return &promotionv1.ListSponsoredSlotsResponse{ListingIds: ids}, nil
}
