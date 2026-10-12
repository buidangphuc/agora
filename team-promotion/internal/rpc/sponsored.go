package rpc

import (
	"context"
	"errors"
	"log/slog"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/auth"

	promotionv1 "github.com/buidangphuc/team-promotion/generated/platform/promotion/v1"
	"github.com/buidangphuc/team-promotion/internal/promotion"
)

// SponsoredHandler serves platform.promotion.v1.SponsoredService: mock
// CreateAdCampaign plus the public ListSponsoredSlots read. No money moves here
// (AGENTS.md §7).
type SponsoredHandler struct {
	promotionv1.UnimplementedSponsoredServiceServer

	svc    *promotion.SponsoredService
	logger *slog.Logger
}

// NewSponsoredHandler constructs the value of the same name.
func NewSponsoredHandler(svc *promotion.SponsoredService, logger *slog.Logger) *SponsoredHandler {
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
	principal, err := auth.RequireScopes(ctx, scopeListingWrite)
	if err != nil {
		return nil, err
	}
	if req.GetListingId() == "" {
		return nil, status.Error(codes.InvalidArgument, "listing_id is required")
	}
	campaign, err := h.svc.CreateAdCampaign(ctx, promotion.CreateAdCampaignParams{
		SellerID:  principal.ID,
		ListingID: req.GetListingId(),
		Budget:    req.GetBudget(),
		Bid:       req.GetBid(),
		IsAdmin:   isAdmin(principal),
	})
	if err != nil {
		if _, ok := status.FromError(err); ok {
			return nil, err
		}
		if errors.Is(err, promotion.ErrInvalidAdCampaign) {
			return nil, status.Error(codes.InvalidArgument, "invalid ad campaign")
		}
		return nil, internalError(ctx, h.logger, "create ad campaign", err)
	}
	return &promotionv1.CreateAdCampaignResponse{Campaign: promotion.AdCampaignToProto(campaign)}, nil
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
