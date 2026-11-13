package rpc

import (
	"context"
	"errors"
	"log/slog"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	promotionv1 "github.com/buidangphuc/team-promotion/generated/platform/promotion/v1"
	"github.com/buidangphuc/team-promotion/internal/sponsored"
	"github.com/buidangphuc/team-promotion/internal/store"
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
		IsAdmin:   isPromotionAdmin(principal),
	})
	if err != nil {
		return nil, fail(ctx, h.logger, err)
	}
	return &promotionv1.CreateAdCampaignResponse{Campaign: adCampaignToProto(campaign)}, nil
}

func (h *SponsoredHandler) ListSponsoredSlots(ctx context.Context, req *promotionv1.ListSponsoredSlotsRequest) (*promotionv1.ListSponsoredSlotsResponse, error) {
	res, err := h.svc.ListSponsoredSlots(ctx, sponsored.SlotsParams{
		ContextStr: req.GetContextStr(),
		Placement:  req.GetPlacement(),
		CategoryID: req.GetCategoryId(),
		Limit:      req.GetLimit(),
	})
	if err != nil {
		return nil, fail(ctx, h.logger, err)
	}
	out := &promotionv1.ListSponsoredSlotsResponse{ListingIds: res.ListingIDs}
	for _, sl := range res.Slots {
		out.Slots = append(out.Slots, &promotionv1.SponsoredSlot{ListingId: sl.ListingID, CampaignId: sl.CampaignID, ImpressionToken: sl.Token})
	}
	return out, nil
}

func (h *SponsoredHandler) RecordAdClick(ctx context.Context, req *promotionv1.RecordAdClickRequest) (*promotionv1.RecordAdClickResponse, error) {
	if req.GetImpressionToken() == "" {
		return nil, status.Error(codes.InvalidArgument, "impression_token is required")
	}
	principalID := ""
	if p, ok := auth.FromContext(ctx); ok && !p.Anonymous() {
		principalID = p.ID
	}
	res, err := h.svc.RecordAdClick(ctx, sponsored.RecordClickParams{Token: req.GetImpressionToken(), PrincipalID: principalID})
	if err != nil {
		if errors.Is(err, store.ErrImpressionNotFound) {
			return nil, status.Error(codes.NotFound, "impression not found")
		}
		return nil, fail(ctx, h.logger, err)
	}
	return &promotionv1.RecordAdClickResponse{ListingId: res.ListingID, Charged: res.Charged}, nil
}
