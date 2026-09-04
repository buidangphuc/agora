package handler

import (
	"context"
	"errors"
	"log/slog"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	promotionv1 "github.com/buidangphuc/team-promotion/generated/platform/promotion/v1"
	"github.com/buidangphuc/team-promotion/internal/interceptor"
	"github.com/buidangphuc/team-promotion/internal/service"
)

// SponsoredHandler serves platform.promotion.v1.SponsoredService: mock
// CreateAdCampaign plus the public ListSponsoredSlots read. No money moves here
// (AGENTS.md §7).
type SponsoredHandler struct {
	promotionv1.UnimplementedSponsoredServiceServer

	svc    *service.SponsoredService
	logger *slog.Logger
}

func NewSponsoredHandler(svc *service.SponsoredService, logger *slog.Logger) *SponsoredHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &SponsoredHandler{svc: svc, logger: logger}
}

// CreateAdCampaign records a sponsored campaign for the authenticated seller
// (MOCK — no charge). The seller id is bound from the principal, never the wire,
// so a caller can only create campaigns for themselves.
func (h *SponsoredHandler) CreateAdCampaign(ctx context.Context, req *promotionv1.CreateAdCampaignRequest) (*promotionv1.CreateAdCampaignResponse, error) {
	principal, err := interceptor.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetListingId() == "" {
		return nil, status.Error(codes.InvalidArgument, "listing_id is required")
	}
	campaign, err := h.svc.CreateAdCampaign(ctx, service.CreateAdCampaignParams{
		SellerID:  principal.GetId(),
		ListingID: req.GetListingId(),
		Budget:    req.GetBudget(),
		Bid:       req.GetBid(),
	})
	if err != nil {
		if errors.Is(err, service.ErrInvalidAdCampaign) {
			return nil, status.Error(codes.InvalidArgument, "invalid ad campaign")
		}
		return nil, status.Errorf(codes.Internal, "create ad campaign: %v", err)
	}
	return &promotionv1.CreateAdCampaignResponse{Campaign: service.AdCampaignToProto(campaign)}, nil
}

// ListSponsoredSlots returns the sponsored listing ids for a placement context,
// best-bid first. Public read (no auth required — mirrors the unauthenticated
// catalog reads): the result carries only listing ids to resolve elsewhere.
func (h *SponsoredHandler) ListSponsoredSlots(ctx context.Context, req *promotionv1.ListSponsoredSlotsRequest) (*promotionv1.ListSponsoredSlotsResponse, error) {
	listingIDs, err := h.svc.ListSponsoredSlots(ctx, req.GetContextStr())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list sponsored slots: %v", err)
	}
	return &promotionv1.ListSponsoredSlotsResponse{ListingIds: listingIDs}, nil
}
