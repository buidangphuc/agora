package handler

import (
	"context"
	"errors"
	"log/slog"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	promotionv1 "github.com/buidangphuc/team-promotion/generated/platform/promotion/v1"
	"github.com/buidangphuc/team-promotion/internal/interceptor"
	"github.com/buidangphuc/team-promotion/internal/repository"
	"github.com/buidangphuc/team-promotion/internal/service"
)

// VoucherHandler serves platform.promotion.v1.VoucherService: CRUD plus the
// idempotent ValidateAndReserve → Commit/Release redemption seam.
type VoucherHandler struct {
	promotionv1.UnimplementedVoucherServiceServer

	svc    *service.VoucherService
	logger *slog.Logger
}

func NewVoucherHandler(svc *service.VoucherService, logger *slog.Logger) *VoucherHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &VoucherHandler{svc: svc, logger: logger}
}

func (h *VoucherHandler) CreateVoucher(ctx context.Context, req *promotionv1.CreateVoucherRequest) (*promotionv1.CreateVoucherResponse, error) {
	principal, err := interceptor.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetCode() == "" {
		return nil, status.Error(codes.InvalidArgument, "code is required")
	}
	// Shop-scoped vouchers belong to the authenticated seller; the wire request
	// carries no seller_id, so it is bound from the principal.
	sellerID := ""
	if req.GetScope() == promotionv1.VoucherScope_VOUCHER_SCOPE_SHOP {
		sellerID = principal.GetId()
	}
	v, err := h.svc.CreateVoucher(ctx, service.CreateVoucherParams{
		Code:          req.GetCode(),
		Scope:         int32(req.GetScope()),
		SellerID:      sellerID,
		DiscountType:  int32(req.GetDiscountType()),
		DiscountValue: req.GetDiscountValue(),
		MinSpend:      req.GetMinSpend(),
		MaxDiscount:   req.GetMaxDiscount(),
		Quota:         req.GetQuota(),
		StartsAt:      service.TimeFromProto(req.GetStartsAt()),
		EndsAt:        service.TimeFromProto(req.GetEndsAt()),
	})
	if err != nil {
		if errors.Is(err, service.ErrInvalidVoucher) {
			return nil, status.Error(codes.InvalidArgument, "invalid voucher")
		}
		return nil, status.Errorf(codes.Internal, "create voucher: %v", err)
	}
	return &promotionv1.CreateVoucherResponse{Voucher: service.VoucherToProto(v)}, nil
}

func (h *VoucherHandler) GetVoucher(ctx context.Context, req *promotionv1.GetVoucherRequest) (*promotionv1.GetVoucherResponse, error) {
	if req.GetCode() == "" {
		return nil, status.Error(codes.InvalidArgument, "code is required")
	}
	v, err := h.svc.GetVoucher(ctx, req.GetCode())
	if err != nil {
		if errors.Is(err, repository.ErrVoucherNotFound) {
			return nil, status.Error(codes.NotFound, "voucher not found")
		}
		return nil, status.Errorf(codes.Internal, "get voucher: %v", err)
	}
	return &promotionv1.GetVoucherResponse{Voucher: service.VoucherToProto(v)}, nil
}

func (h *VoucherHandler) ListVouchers(ctx context.Context, req *promotionv1.ListVouchersRequest) (*promotionv1.ListVouchersResponse, error) {
	cursor := req.GetPage().GetCursor()
	pageSize := req.GetPage().GetPageSize()
	items, next, err := h.svc.ListVouchers(ctx, req.GetSellerId(), cursor, pageSize)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list vouchers: %v", err)
	}
	out := make([]*promotionv1.Voucher, 0, len(items))
	for _, v := range items {
		out = append(out, service.VoucherToProto(v))
	}
	return &promotionv1.ListVouchersResponse{
		Vouchers: out,
		Page:     pageResponse(next),
	}, nil
}

func (h *VoucherHandler) ValidateAndReserve(ctx context.Context, req *promotionv1.ValidateAndReserveRequest) (*promotionv1.ValidateAndReserveResponse, error) {
	if req.GetReservationId() == "" {
		return nil, status.Error(codes.InvalidArgument, "reservation_id is required")
	}
	res, err := h.svc.ValidateAndReserve(ctx, req.GetReservationId(), req.GetCode(), req.GetBuyerId(), req.GetCartSubtotal(), req.GetSellerId())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "validate and reserve: %v", err)
	}
	return &promotionv1.ValidateAndReserveResponse{
		Valid:          res.Valid,
		Reason:         res.Reason,
		DiscountAmount: res.DiscountAmount,
		VoucherId:      res.VoucherID,
	}, nil
}

func (h *VoucherHandler) CommitReservation(ctx context.Context, req *promotionv1.CommitReservationRequest) (*promotionv1.CommitReservationResponse, error) {
	if req.GetReservationId() == "" {
		return nil, status.Error(codes.InvalidArgument, "reservation_id is required")
	}
	committed, err := h.svc.CommitReservation(ctx, req.GetReservationId())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "commit reservation: %v", err)
	}
	return &promotionv1.CommitReservationResponse{Committed: committed}, nil
}

func (h *VoucherHandler) ReleaseReservation(ctx context.Context, req *promotionv1.ReleaseReservationRequest) (*promotionv1.ReleaseReservationResponse, error) {
	if req.GetReservationId() == "" {
		return nil, status.Error(codes.InvalidArgument, "reservation_id is required")
	}
	released, err := h.svc.ReleaseReservation(ctx, req.GetReservationId())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "release reservation: %v", err)
	}
	return &promotionv1.ReleaseReservationResponse{Released: released}, nil
}
