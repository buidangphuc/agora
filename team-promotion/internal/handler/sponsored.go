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
// (MOCK — no charge). Requires listing.write and, for non-admins, ownership of the
// listing (verified against team-domain in the service). The seller id is bound
// from the principal, never the wire.
func (h *SponsoredHandler) CreateAdCampaign(ctx context.Context, req *promotionv1.CreateAdCampaignRequest) (*promotionv1.CreateAdCampaignResponse, error) {
	principal, err := interceptor.RequireScopes(ctx, interceptor.ScopeListingWrite)
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
		IsAdmin:   interceptor.IsAdmin(principal),
	})
	if err != nil {
		if _, ok := status.FromError(err); ok {
			return nil, err
		}
		if errors.Is(err, service.ErrInvalidAdCampaign) {
			return nil, status.Error(codes.InvalidArgument, "invalid ad campaign")
		}
		return nil, internalError(ctx, h.logger, "create ad campaign", err)
	}
	return &promotionv1.CreateAdCampaignResponse{Campaign: service.AdCampaignToProto(campaign)}, nil
}

// ListSponsoredSlots returns the sponsored listing ids for a placement context,
// best-bid first. Public read (no auth required — mirrors the unauthenticated
// catalog reads): the result carries only listing ids to resolve elsewhere.
func (h *SponsoredHandler) ListSponsoredSlots(ctx context.Context, req *promotionv1.ListSponsoredSlotsRequest) (*promotionv1.ListSponsoredSlotsResponse, error) {
	listingIDs, err := h.svc.ListSponsoredSlots(ctx, req.GetContextStr())
	if err != nil {
		return nil, internalError(ctx, h.logger, "list sponsored slots", err)
	}
	return &promotionv1.ListSponsoredSlotsResponse{ListingIds: listingIDs}, nil
}
