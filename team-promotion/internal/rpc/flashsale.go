package rpc

import (
	"context"
	"log/slog"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	promotionv1 "github.com/buidangphuc/team-promotion/generated/platform/promotion/v1"
	"github.com/buidangphuc/team-promotion/internal/flashsale"
)

const maxReservationIDLen = 200

type FlashSaleHandler struct {
	promotionv1.UnimplementedFlashSaleServiceServer

	svc    *flashsale.Service
	logger *slog.Logger
}

func NewFlashSaleHandler(svc *flashsale.Service, logger *slog.Logger) *FlashSaleHandler {
	return &FlashSaleHandler{svc: svc, logger: orDefault(logger)}
}

func (h *FlashSaleHandler) CreateCampaign(ctx context.Context, req *promotionv1.CreateCampaignRequest) (*promotionv1.CreateCampaignResponse, error) {
	principal, err := requireSeller(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetListingId() == "" {
		return nil, status.Error(codes.InvalidArgument, "listing_id is required")
	}
	c, err := h.svc.CreateCampaign(ctx, flashsale.CreateCampaignParams{
		ListingID: req.GetListingId(),
		VariantID: req.GetVariantId(),
		SalePrice: req.GetSalePrice(),
		StockCap:  req.GetStockCap(),
		StartsAt:  timeFromProto(req.GetStartsAt()),
		EndsAt:    timeFromProto(req.GetEndsAt()),
		CallerID:  principal.ID,
		IsAdmin:   principal.HasScope(scopeAdmin),
	})
	if err != nil {
		return nil, fail(ctx, h.logger, err)
	}
	return &promotionv1.CreateCampaignResponse{Campaign: campaignToProto(c)}, nil
}

func (h *FlashSaleHandler) GetActiveFlashSale(ctx context.Context, req *promotionv1.GetActiveFlashSaleRequest) (*promotionv1.GetActiveFlashSaleResponse, error) {
	if req.GetListingId() == "" {
		return nil, status.Error(codes.InvalidArgument, "listing_id is required")
	}
	c, active, err := h.svc.GetActiveFlashSale(ctx, req.GetListingId())
	if err != nil {
		return nil, fail(ctx, h.logger, err)
	}
	resp := &promotionv1.GetActiveFlashSaleResponse{Active: active}
	if active {
		resp.Campaign = campaignToProto(c)
	}
	return resp, nil
}

func (h *FlashSaleHandler) ListActiveCampaigns(ctx context.Context, req *promotionv1.ListActiveCampaignsRequest) (*promotionv1.ListActiveCampaignsResponse, error) {
	items, next, err := h.svc.ListActiveCampaigns(ctx, req.GetPage().GetCursor(), req.GetPage().GetPageSize())
	if err != nil {
		return nil, fail(ctx, h.logger, err)
	}
	out := make([]*promotionv1.FlashSaleCampaign, 0, len(items))
	for _, c := range items {
		out = append(out, campaignToProto(c))
	}
	return &promotionv1.ListActiveCampaignsResponse{Campaigns: out, Page: pageResponse(next)}, nil
}

func (h *FlashSaleHandler) GetFlashSaleStock(ctx context.Context, req *promotionv1.GetFlashSaleStockRequest) (*promotionv1.GetFlashSaleStockResponse, error) {
	if req.GetCampaignId() == "" {
		return nil, status.Error(codes.InvalidArgument, "campaign_id is required")
	}
	remaining, stockCap, err := h.svc.GetFlashSaleStock(ctx, req.GetCampaignId())
	if err != nil {
		return nil, fail(ctx, h.logger, err)
	}
	return &promotionv1.GetFlashSaleStockResponse{Remaining: remaining, StockCap: stockCap}, nil
}

func (h *FlashSaleHandler) ReserveFlashSaleStock(ctx context.Context, req *promotionv1.ReserveFlashSaleStockRequest) (*promotionv1.ReserveFlashSaleStockResponse, error) {
	if _, err := requireService(ctx, scopePromoReserve); err != nil {
		return nil, err
	}
	if err := validReservationID(req.GetReservationId()); err != nil {
		return nil, err
	}
	if req.GetCampaignId() == "" {
		return nil, status.Error(codes.InvalidArgument, "campaign_id is required")
	}
	if req.GetListingId() == "" {
		return nil, status.Error(codes.InvalidArgument, "listing_id is required")
	}
	if req.GetQuantity() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "quantity must be positive")
	}
	reserved, reason, err := h.svc.ReserveFlashSaleStock(ctx, req.GetReservationId(), req.GetCampaignId(),
		req.GetListingId(), req.GetVariantId(), req.GetQuantity(), req.GetExpectedSalePrice())
	if err != nil {
		return nil, fail(ctx, h.logger, err)
	}
	return &promotionv1.ReserveFlashSaleStockResponse{Reserved: reserved, Reason: reason}, nil
}

func (h *FlashSaleHandler) CommitFlashSaleStock(ctx context.Context, req *promotionv1.CommitFlashSaleStockRequest) (*promotionv1.CommitFlashSaleStockResponse, error) {
	if _, err := requireService(ctx, scopePromoReserve); err != nil {
		return nil, err
	}
	if err := validReservationID(req.GetReservationId()); err != nil {
		return nil, err
	}
	ok, err := h.svc.CommitFlashSaleStock(ctx, req.GetReservationId())
	if err != nil {
		return nil, fail(ctx, h.logger, err)
	}
	return &promotionv1.CommitFlashSaleStockResponse{Committed: ok}, nil
}

func (h *FlashSaleHandler) ReleaseFlashSaleStock(ctx context.Context, req *promotionv1.ReleaseFlashSaleStockRequest) (*promotionv1.ReleaseFlashSaleStockResponse, error) {
	if _, err := requireService(ctx, scopePromoReserve); err != nil {
		return nil, err
	}
	if err := validReservationID(req.GetReservationId()); err != nil {
		return nil, err
	}
	ok, err := h.svc.ReleaseFlashSaleStock(ctx, req.GetReservationId())
	if err != nil {
		return nil, fail(ctx, h.logger, err)
	}
	return &promotionv1.ReleaseFlashSaleStockResponse{Released: ok}, nil
}

func validReservationID(id string) error {
	if id == "" {
		return status.Error(codes.InvalidArgument, "reservation_id is required")
	}
	if len(id) > maxReservationIDLen {
		return status.Error(codes.InvalidArgument, "reservation_id too long")
	}
	return nil
}
