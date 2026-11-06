package rpc

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	promotionv1 "github.com/buidangphuc/team-promotion/generated/platform/promotion/v1"
	"github.com/buidangphuc/team-promotion/internal/store"
	"github.com/buidangphuc/team-promotion/internal/voucher"
)

const PreviewReservationPrefix = "preview:"

type VoucherHandler struct {
	promotionv1.UnimplementedVoucherServiceServer

	svc    *voucher.Service
	logger *slog.Logger
}

func NewVoucherHandler(svc *voucher.Service, logger *slog.Logger) *VoucherHandler {
	return &VoucherHandler{svc: svc, logger: orDefault(logger)}
}

func (h *VoucherHandler) CreateVoucher(ctx context.Context, req *promotionv1.CreateVoucherRequest) (*promotionv1.CreateVoucherResponse, error) {
	if _, err := auth.RequirePrincipal(ctx); err != nil {
		return nil, err
	}
	if req.GetCode() == "" {
		return nil, status.Error(codes.InvalidArgument, "code is required")
	}
	var principal auth.Principal
	var err error
	sellerID := ""
	auditAdmin := false
	if req.GetScope() == promotionv1.VoucherScope_VOUCHER_SCOPE_SHOP {
		principal, err = requireSeller(ctx)
		if err != nil {
			return nil, err
		}
		sellerID = principal.ID
		auditAdmin = isPromotionAdmin(principal)
	} else {
		principal, err = requireAdmin(ctx)
		if err != nil {
			return nil, err
		}
	}
	v, err := h.svc.CreateVoucher(ctx, voucher.CreateVoucherParams{
		Code:          req.GetCode(),
		Scope:         int32(req.GetScope()),
		SellerID:      sellerID,
		DiscountType:  int32(req.GetDiscountType()),
		DiscountValue: req.GetDiscountValue(),
		MinSpend:      req.GetMinSpend(),
		MaxDiscount:   req.GetMaxDiscount(),
		Quota:         req.GetQuota(),
		StartsAt:      timeFromProto(req.GetStartsAt()),
		EndsAt:        timeFromProto(req.GetEndsAt()),
		PerUserLimit:  req.GetPerUserLimit(),
		NewUserOnly:   req.GetNewUserOnly(),
		AuditActorID:  principal.ID,
		AuditAdmin:    auditAdmin,
	})
	if err != nil {
		return nil, fail(ctx, h.logger, err)
	}
	return &promotionv1.CreateVoucherResponse{Voucher: voucherToProto(v)}, nil
}

func (h *VoucherHandler) DeactivateVoucher(ctx context.Context, req *promotionv1.DeactivateVoucherRequest) (*promotionv1.DeactivateVoucherResponse, error) {
	principal, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
	}
	v, err := h.svc.DeactivateVoucher(ctx, voucher.DeactivateVoucherParams{
		VoucherID:    req.GetVoucherId(),
		AuditActorID: principal.ID,
	})
	if err != nil {
		return nil, fail(ctx, h.logger, err)
	}
	return &promotionv1.DeactivateVoucherResponse{Voucher: voucherToProto(v)}, nil
}

func (h *VoucherHandler) UpdateVoucher(ctx context.Context, req *promotionv1.UpdateVoucherRequest) (*promotionv1.UpdateVoucherResponse, error) {
	principal, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
	}
	p := voucher.UpdateVoucherParams{
		VoucherID:    req.GetVoucherId(),
		Quota:        req.Quota,
		PerUserLimit: req.PerUserLimit,
		NewUserOnly:  req.NewUserOnly,
		AuditActorID: principal.ID,
	}
	if req.EndsAt != nil {
		t := req.GetEndsAt().AsTime()
		p.EndsAt = &t
	}
	v, err := h.svc.UpdateVoucher(ctx, p)
	if err != nil {
		if errors.Is(err, voucher.ErrInvalidVoucher) {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		return nil, fail(ctx, h.logger, err)
	}
	return &promotionv1.UpdateVoucherResponse{Voucher: voucherToProto(v)}, nil
}

func (h *VoucherHandler) GetVoucher(ctx context.Context, req *promotionv1.GetVoucherRequest) (*promotionv1.GetVoucherResponse, error) {
	if req.GetCode() == "" {
		return nil, status.Error(codes.InvalidArgument, "code is required")
	}
	v, err := h.svc.GetVoucher(ctx, req.GetCode())
	if err != nil {
		return nil, fail(ctx, h.logger, err)
	}
	if v.OwnerUserID != "" {
		if p, ok := auth.FromContext(ctx); !ok || p.Anonymous() || p.ID != v.OwnerUserID {
			return nil, fail(ctx, h.logger, store.ErrVoucherNotFound)
		}
	}
	return &promotionv1.GetVoucherResponse{Voucher: voucherToProto(v)}, nil
}

func (h *VoucherHandler) ListMyVouchers(ctx context.Context, _ *promotionv1.ListMyVouchersRequest) (*promotionv1.ListMyVouchersResponse, error) {
	p, err := auth.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	items, err := h.svc.ListMyVouchers(ctx, p.ID)
	if err != nil {
		return nil, fail(ctx, h.logger, err)
	}
	out := make([]*promotionv1.Voucher, 0, len(items))
	for _, v := range items {
		out = append(out, voucherToProto(v))
	}
	return &promotionv1.ListMyVouchersResponse{Vouchers: out}, nil
}

func (h *VoucherHandler) ListVouchers(ctx context.Context, req *promotionv1.ListVouchersRequest) (*promotionv1.ListVouchersResponse, error) {
	items, next, err := h.svc.ListVouchers(ctx, req.GetSellerId(), req.GetPage().GetCursor(), req.GetPage().GetPageSize())
	if err != nil {
		return nil, fail(ctx, h.logger, err)
	}
	out := make([]*promotionv1.Voucher, 0, len(items))
	for _, v := range items {
		out = append(out, voucherToProto(v))
	}
	return &promotionv1.ListVouchersResponse{Vouchers: out, Page: pageResponse(next)}, nil
}

func (h *VoucherHandler) ValidateAndReserve(ctx context.Context, req *promotionv1.ValidateAndReserveRequest) (*promotionv1.ValidateAndReserveResponse, error) {
	p, err := auth.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	buyerID := req.GetBuyerId()
	if p.Type == auth.TypeService {
		if _, err := requireService(ctx, scopePromoReserve); err != nil {
			return nil, err
		}
	} else {
		buyerID = p.ID
		if !strings.HasPrefix(req.GetReservationId(), PreviewReservationPrefix+p.ID+":") {
			return nil, status.Error(codes.PermissionDenied, "reservation_id outside the caller's preview namespace")
		}
	}
	if req.GetReservationId() == "" {
		return nil, status.Error(codes.InvalidArgument, "reservation_id is required")
	}
	res, err := h.svc.ValidateAndReserve(ctx, req.GetReservationId(), req.GetCode(), buyerID, req.GetCartSubtotal(), req.GetSellerId())
	if err != nil {
		return nil, fail(ctx, h.logger, err)
	}
	return &promotionv1.ValidateAndReserveResponse{
		Valid:          res.Valid,
		Reason:         res.Reason,
		DiscountAmount: res.DiscountAmount,
		VoucherId:      res.VoucherID,
		Scope:          promotionv1.VoucherScope(res.Scope),
		SellerId:       res.SellerID,
	}, nil
}

func (h *VoucherHandler) CommitReservation(ctx context.Context, req *promotionv1.CommitReservationRequest) (*promotionv1.CommitReservationResponse, error) {
	if _, err := requireService(ctx, scopePromoReserve); err != nil {
		return nil, err
	}
	if req.GetReservationId() == "" {
		return nil, status.Error(codes.InvalidArgument, "reservation_id is required")
	}
	committed, err := h.svc.CommitReservation(ctx, req.GetReservationId())
	if err != nil {
		return nil, fail(ctx, h.logger, err)
	}
	return &promotionv1.CommitReservationResponse{Committed: committed}, nil
}

func (h *VoucherHandler) ReleaseReservation(ctx context.Context, req *promotionv1.ReleaseReservationRequest) (*promotionv1.ReleaseReservationResponse, error) {
	if _, err := requireService(ctx, scopePromoReserve); err != nil {
		return nil, err
	}
	if req.GetReservationId() == "" {
		return nil, status.Error(codes.InvalidArgument, "reservation_id is required")
	}
	released, err := h.svc.ReleaseReservation(ctx, req.GetReservationId())
	if err != nil {
		return nil, fail(ctx, h.logger, err)
	}
	return &promotionv1.ReleaseReservationResponse{Released: released}, nil
}
