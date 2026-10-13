package rpc

import (
	"context"
	"log/slog"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/auth"

	commonv1 "github.com/buidangphuc/team-promotion/generated/platform/common/v1"
	promotionv1 "github.com/buidangphuc/team-promotion/generated/platform/promotion/v1"
	"github.com/buidangphuc/team-promotion/internal/promotion"
)

// FlashSaleHandler serves platform.promotion.v1.FlashSaleService.
type FlashSaleHandler struct {
	promotionv1.UnimplementedFlashSaleServiceServer

	svc    *promotion.FlashSaleService
	logger *slog.Logger
}

// NewFlashSaleHandler constructs the value of the same name.
func NewFlashSaleHandler(svc *promotion.FlashSaleService, logger *slog.Logger) *FlashSaleHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &FlashSaleHandler{svc: svc, logger: logger}
}

// CreateCampaign implements the repository contract for FlashSaleHandler.
func (h *FlashSaleHandler) CreateCampaign(ctx context.Context, req *promotionv1.CreateCampaignRequest) (*promotionv1.CreateCampaignResponse, error) {
	principal, err := auth.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	// Sellers and admins only. Non-admin sellers must own the listing; the service
	// verifies that against team-domain.
	if err := requireSeller(principal); err != nil {
		return nil, err
	}
	if req.GetListingId() == "" {
		return nil, status.Error(codes.InvalidArgument, "listing_id is required")
	}
	c, err := h.svc.CreateCampaign(ctx, promotion.CreateCampaignParams{
		ListingID: req.GetListingId(),
		VariantID: req.GetVariantId(),
		SalePrice: req.GetSalePrice(),
		StockCap:  req.GetStockCap(),
		StartsAt:  promotion.TimeFromProto(req.GetStartsAt()),
		EndsAt:    promotion.TimeFromProto(req.GetEndsAt()),
		CallerID:  principal.ID,
		IsAdmin:   isAdmin(principal),
	})
	if err != nil {
		return nil, internalError(ctx, h.logger, "create campaign", err)
	}
	return &promotionv1.CreateCampaignResponse{Campaign: promotion.CampaignToProto(c)}, nil
}

// GetActiveFlashSale implements the repository contract for FlashSaleHandler.
func (h *FlashSaleHandler) GetActiveFlashSale(ctx context.Context, req *promotionv1.GetActiveFlashSaleRequest) (*promotionv1.GetActiveFlashSaleResponse, error) {
	if req.GetListingId() == "" {
		return nil, status.Error(codes.InvalidArgument, "listing_id is required")
	}
	c, active, err := h.svc.GetActiveFlashSale(ctx, req.GetListingId())
	if err != nil {
		return nil, internalError(ctx, h.logger, "get active flash sale", err)
	}
	resp := &promotionv1.GetActiveFlashSaleResponse{Active: active}
	if active {
		resp.Campaign = promotion.CampaignToProto(c)
	}
	return resp, nil
}

// ListActiveCampaigns implements the repository contract for FlashSaleHandler.
func (h *FlashSaleHandler) ListActiveCampaigns(ctx context.Context, req *promotionv1.ListActiveCampaignsRequest) (*promotionv1.ListActiveCampaignsResponse, error) {
	items, next, err := h.svc.ListActiveCampaigns(ctx, req.GetPage().GetCursor(), req.GetPage().GetPageSize())
	if err != nil {
		return nil, internalError(ctx, h.logger, "list active campaigns", err)
	}
	out := make([]*promotionv1.FlashSaleCampaign, 0, len(items))
	for _, c := range items {
		out = append(out, promotion.CampaignToProto(c))
	}
	return &promotionv1.ListActiveCampaignsResponse{
		Campaigns: out,
		Page:      pageResponse(next),
	}, nil
}

// GetFlashSaleStock implements the repository contract for FlashSaleHandler.
func (h *FlashSaleHandler) GetFlashSaleStock(ctx context.Context, req *promotionv1.GetFlashSaleStockRequest) (*promotionv1.GetFlashSaleStockResponse, error) {
	if req.GetCampaignId() == "" {
		return nil, status.Error(codes.InvalidArgument, "campaign_id is required")
	}
	remaining, stockCap, err := h.svc.GetFlashSaleStock(ctx, req.GetCampaignId())
	if err != nil {
		return nil, internalError(ctx, h.logger, "get flash sale stock", err)
	}
	return &promotionv1.GetFlashSaleStockResponse{Remaining: remaining, StockCap: stockCap}, nil
}

// pageResponse builds a PageResponse carrying the next cursor. Total is left as 0
// (best-effort/unknown) since the repositories do not compute an exact count.
func pageResponse(next string) *commonv1.PageResponse {
	return &commonv1.PageResponse{NextCursor: next}
}
