package rpc

import (
	"context"
	"errors"
	"log/slog"
	"regexp"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	promotionv1 "github.com/buidangphuc/team-promotion/generated/platform/promotion/v1"
	"github.com/buidangphuc/team-promotion/internal/flashsale"
	"github.com/buidangphuc/team-promotion/internal/store"
)

const maxReservationIDLen = 200

var idempotencyKeyPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{8,64}$`)

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
	isAdmin := isPromotionAdmin(principal)
	if isAdmin && !idempotencyKeyPattern.MatchString(req.GetIdempotencyKey()) {
		return nil, status.Error(codes.InvalidArgument, "idempotency_key must be 8-64 characters of letters, digits, '_' or '-'")
	}
	c, err := h.svc.CreateCampaign(ctx, flashsale.CreateCampaignParams{
		ListingID: req.GetListingId(),
		VariantID: req.GetVariantId(),
		SalePrice: req.GetSalePrice(),
		StockCap:  req.GetStockCap(),
		StartsAt:  timeFromProto(req.GetStartsAt()),
		EndsAt:    timeFromProto(req.GetEndsAt()),
		CallerID:  principal.ID,
		IsAdmin:   isAdmin,

		IdempotencyKey: req.GetIdempotencyKey(),
	})
	if err != nil {
		return nil, fail(ctx, h.logger, err)
	}
	cp := campaignToProto(c)
	cp.Status = campaignStatusToProto(h.svc.StatusOf(c))
	return &promotionv1.CreateCampaignResponse{Campaign: cp}, nil
}

func (h *FlashSaleHandler) ListCampaigns(ctx context.Context, req *promotionv1.ListCampaignsRequest) (*promotionv1.ListCampaignsResponse, error) {
	if _, err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	statuses := make([]string, 0, len(req.GetStatuses()))
	for _, st := range req.GetStatuses() {
		if v := campaignStatusFromProto(st); v != "" {
			statuses = append(statuses, v)
		}
	}
	res, err := h.svc.ListCampaigns(ctx, statuses, req.GetPage().GetCursor(), req.GetPage().GetPageSize())
	if err != nil {
		return nil, fail(ctx, h.logger, err)
	}
	out := make([]*promotionv1.FlashSaleCampaign, 0, len(res.Campaigns))
	for _, v := range res.Campaigns {
		cp := campaignToProto(v.FlashSaleCampaign)
		cp.Status = campaignStatusToProto(v.Status)
		out = append(out, cp)
	}
	return &promotionv1.ListCampaignsResponse{Campaigns: out, Page: pageResponse(res.NextCursor), FlashSaleDisabled: res.FlashSaleDisabled}, nil
}

func (h *FlashSaleHandler) CancelCampaign(ctx context.Context, req *promotionv1.CancelCampaignRequest) (*promotionv1.CancelCampaignResponse, error) {
	principal, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetCampaignId() == "" {
		return nil, status.Error(codes.InvalidArgument, "campaign_id is required")
	}
	res, err := h.svc.CancelCampaign(ctx, req.GetCampaignId(), principal.ID)
	if err != nil {
		return nil, fail(ctx, h.logger, err)
	}
	cp := campaignToProto(res.Campaign)
	cp.Status = campaignStatusToProto(res.Status)
	cp.CancelledAt = protoTime(res.CancelledAt)
	return &promotionv1.CancelCampaignResponse{Campaign: cp}, nil
}

func sellerCampaignToProto(c store.FlashSaleCampaign, st string) *promotionv1.FlashSaleCampaign {
	cp := campaignToProto(c)
	cp.SellerId = c.SellerID
	cp.Status = campaignStatusToProto(st)
	return cp
}

func (h *FlashSaleHandler) ListMyCampaigns(ctx context.Context, req *promotionv1.ListMyCampaignsRequest) (*promotionv1.ListMyCampaignsResponse, error) {
	principal, err := auth.RequireScopes(ctx, scopeListingWrite)
	if err != nil {
		return nil, err
	}
	statuses := make([]string, 0, len(req.GetStatuses()))
	for _, st := range req.GetStatuses() {
		if v := campaignStatusFromProto(st); v != "" {
			statuses = append(statuses, v)
		}
	}
	res, err := h.svc.ListMyCampaigns(ctx, principal.ID, statuses, req.GetPage().GetCursor(), req.GetPage().GetPageSize())
	if err != nil {
		return nil, fail(ctx, h.logger, err)
	}
	out := make([]*promotionv1.FlashSaleCampaign, 0, len(res.Campaigns))
	for _, v := range res.Campaigns {
		out = append(out, sellerCampaignToProto(v.FlashSaleCampaign, v.Status))
	}
	return &promotionv1.ListMyCampaignsResponse{Campaigns: out, Page: pageResponse(res.NextCursor)}, nil
}

func (h *FlashSaleHandler) CancelMyCampaign(ctx context.Context, req *promotionv1.CancelMyCampaignRequest) (*promotionv1.CancelMyCampaignResponse, error) {
	principal, err := auth.RequireScopes(ctx, scopeListingWrite)
	if err != nil {
		return nil, err
	}
	if req.GetCampaignId() == "" {
		return nil, status.Error(codes.InvalidArgument, "campaign_id is required")
	}
	res, err := h.svc.CancelMyCampaign(ctx, req.GetCampaignId(), principal.ID)
	if err != nil {
		if errors.Is(err, store.ErrNotOwner) {
			return nil, status.Error(codes.PermissionDenied, "not allowed to cancel this campaign")
		}
		return nil, fail(ctx, h.logger, err)
	}
	cp := sellerCampaignToProto(res.Campaign, res.Status)
	cp.CancelledAt = protoTime(res.CancelledAt)
	return &promotionv1.CancelMyCampaignResponse{Campaign: cp}, nil
}

func (h *FlashSaleHandler) UpdateCampaign(ctx context.Context, req *promotionv1.UpdateCampaignRequest) (*promotionv1.UpdateCampaignResponse, error) {
	principal, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetCampaignId() == "" {
		return nil, status.Error(codes.InvalidArgument, "campaign_id is required")
	}
	c, err := h.svc.UpdateCampaign(ctx, flashsale.UpdateCampaignParams{
		CampaignID: req.GetCampaignId(),
		SalePrice:  req.GetSalePrice(),
		StockCap:   req.GetStockCap(),
		StartsAt:   timeFromProto(req.GetStartsAt()),
		EndsAt:     timeFromProto(req.GetEndsAt()),
		ActorID:    principal.ID,
	})
	if err != nil {
		return nil, fail(ctx, h.logger, err)
	}
	cp := campaignToProto(c)
	cp.Status = campaignStatusToProto(store.CampaignStatusScheduled)
	return &promotionv1.UpdateCampaignResponse{Campaign: cp}, nil
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
	res, err := h.svc.ListActiveCampaignsPage(ctx, req.GetPage().GetCursor(), req.GetPage().GetPageSize())
	if err != nil {
		return nil, fail(ctx, h.logger, err)
	}
	out := make([]*promotionv1.FlashSaleCampaign, 0, len(res.Campaigns))
	for _, c := range res.Campaigns {
		out = append(out, campaignToProto(c))
	}
	return &promotionv1.ListActiveCampaignsResponse{Campaigns: out, Page: pageResponse(res.NextCursor), FlashSaleDisabled: res.FlashSaleDisabled}, nil
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
