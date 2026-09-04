package handler

import (
	"context"
	"errors"
	"log/slog"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	commonv1 "github.com/buidangphuc/team-promotion/generated/platform/common/v1"
	promotionv1 "github.com/buidangphuc/team-promotion/generated/platform/promotion/v1"
	"github.com/buidangphuc/team-promotion/internal/interceptor"
	"github.com/buidangphuc/team-promotion/internal/repository"
	"github.com/buidangphuc/team-promotion/internal/service"
)

// FlashSaleHandler serves platform.promotion.v1.FlashSaleService.
type FlashSaleHandler struct {
	promotionv1.UnimplementedFlashSaleServiceServer

	svc    *service.FlashSaleService
	logger *slog.Logger
}

func NewFlashSaleHandler(svc *service.FlashSaleService, logger *slog.Logger) *FlashSaleHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &FlashSaleHandler{svc: svc, logger: logger}
}

func (h *FlashSaleHandler) CreateCampaign(ctx context.Context, req *promotionv1.CreateCampaignRequest) (*promotionv1.CreateCampaignResponse, error) {
	if _, err := interceptor.RequirePrincipal(ctx); err != nil {
		return nil, err
	}
	if req.GetListingId() == "" {
		return nil, status.Error(codes.InvalidArgument, "listing_id is required")
	}
	c, err := h.svc.CreateCampaign(ctx, service.CreateCampaignParams{
		ListingID: req.GetListingId(),
		VariantID: req.GetVariantId(),
		SalePrice: req.GetSalePrice(),
		StockCap:  req.GetStockCap(),
		StartsAt:  service.TimeFromProto(req.GetStartsAt()),
		EndsAt:    service.TimeFromProto(req.GetEndsAt()),
	})
	if err != nil {
		if errors.Is(err, service.ErrInvalidCampaign) {
			return nil, status.Error(codes.InvalidArgument, "invalid campaign")
		}
		return nil, status.Errorf(codes.Internal, "create campaign: %v", err)
	}
	return &promotionv1.CreateCampaignResponse{Campaign: service.CampaignToProto(c)}, nil
}

func (h *FlashSaleHandler) GetActiveFlashSale(ctx context.Context, req *promotionv1.GetActiveFlashSaleRequest) (*promotionv1.GetActiveFlashSaleResponse, error) {
	if req.GetListingId() == "" {
		return nil, status.Error(codes.InvalidArgument, "listing_id is required")
	}
	c, active, err := h.svc.GetActiveFlashSale(ctx, req.GetListingId())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get active flash sale: %v", err)
	}
	resp := &promotionv1.GetActiveFlashSaleResponse{Active: active}
	if active {
		resp.Campaign = service.CampaignToProto(c)
	}
	return resp, nil
}

func (h *FlashSaleHandler) ListActiveCampaigns(ctx context.Context, req *promotionv1.ListActiveCampaignsRequest) (*promotionv1.ListActiveCampaignsResponse, error) {
	items, next, err := h.svc.ListActiveCampaigns(ctx, req.GetPage().GetCursor(), req.GetPage().GetPageSize())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list active campaigns: %v", err)
	}
	out := make([]*promotionv1.FlashSaleCampaign, 0, len(items))
	for _, c := range items {
		out = append(out, service.CampaignToProto(c))
	}
	return &promotionv1.ListActiveCampaignsResponse{
		Campaigns: out,
		Page:      pageResponse(next),
	}, nil
}

func (h *FlashSaleHandler) GetFlashSaleStock(ctx context.Context, req *promotionv1.GetFlashSaleStockRequest) (*promotionv1.GetFlashSaleStockResponse, error) {
	if req.GetCampaignId() == "" {
		return nil, status.Error(codes.InvalidArgument, "campaign_id is required")
	}
	remaining, stockCap, err := h.svc.GetFlashSaleStock(ctx, req.GetCampaignId())
	if err != nil {
		if errors.Is(err, repository.ErrCampaignNotFound) {
			return nil, status.Error(codes.NotFound, "campaign not found")
		}
		return nil, status.Errorf(codes.Internal, "get flash sale stock: %v", err)
	}
	return &promotionv1.GetFlashSaleStockResponse{Remaining: remaining, StockCap: stockCap}, nil
}

// pageResponse builds a PageResponse carrying the next cursor. Total is left as 0
// (best-effort/unknown) since the repositories do not compute an exact count.
func pageResponse(next string) *commonv1.PageResponse {
	return &commonv1.PageResponse{NextCursor: next}
}
