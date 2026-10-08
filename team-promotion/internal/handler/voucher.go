package handler

import (
	"context"
	"errors"
	"log/slog"
	"strings"

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
	// Platform vouchers are admin-only. Shop vouchers require a seller (or admin)
	// and are always bound to the caller's own id: the wire request carries no
	// seller_id, so a seller can only ever create vouchers for their own shop.
	sellerID := ""
	switch req.GetScope() {
	case promotionv1.VoucherScope_VOUCHER_SCOPE_PLATFORM:
		if err := interceptor.RequireAdmin(principal); err != nil {
			return nil, err
		}
	case promotionv1.VoucherScope_VOUCHER_SCOPE_SHOP:
		if err := interceptor.RequireSeller(principal); err != nil {
			return nil, err
		}
		sellerID = principal.GetId()
	default:
		// Unspecified scope: treat as the most restrictive (platform) until validated downstream.
		if err := interceptor.RequireAdmin(principal); err != nil {
			return nil, err
		}
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
		return nil, internalError(ctx, h.logger, "create voucher", err)
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
		return nil, internalError(ctx, h.logger, "get voucher", err)
	}
	return &promotionv1.GetVoucherResponse{Voucher: service.VoucherToProto(v)}, nil
}

func (h *VoucherHandler) ListVouchers(ctx context.Context, req *promotionv1.ListVouchersRequest) (*promotionv1.ListVouchersResponse, error) {
	cursor := req.GetPage().GetCursor()
	pageSize := req.GetPage().GetPageSize()
	items, next, err := h.svc.ListVouchers(ctx, req.GetSellerId(), cursor, pageSize)
	if err != nil {
		return nil, internalError(ctx, h.logger, "list vouchers", err)
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

// PreviewReservationPrefix namespaces non-service ValidateAndReserve holds: a
// preview reservation_id must be "preview:<caller id>:<anything>".
const PreviewReservationPrefix = "preview:"

// ValidateAndReserve is dual-mode.
//
//   - A SERVICE principal holding promotion.reserve (team-order) is trusted: the
//     request's buyer_id / seller_id / cart_subtotal / reservation_id are honoured.
//   - Any other authenticated principal (the checkout voucher preview, called
//     through the gateway) is bound to itself: buyer_id is the principal id (the
//     request value is ignored) and reservation_id must live in the caller's own
//     "preview:<principal id>:" namespace. Such a hold can never be committed or
//     released by its creator (those RPCs are service-only), so the user path
//     cannot consume quota.
//   - A service principal without promotion.reserve is refused; none/anonymous is
//     UNAUTHENTICATED.
func (h *VoucherHandler) ValidateAndReserve(ctx context.Context, req *promotionv1.ValidateAndReserveRequest) (*promotionv1.ValidateAndReserveResponse, error) {
	p, err := interceptor.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	buyerID := req.GetBuyerId()
	if interceptor.IsService(p) {
		if _, err := interceptor.RequireService(ctx, interceptor.ScopePromoReserve); err != nil {
			return nil, err
		}
	} else {
		buyerID = p.GetId()
		if !strings.HasPrefix(req.GetReservationId(), PreviewReservationPrefix+p.GetId()+":") {
			return nil, status.Error(codes.PermissionDenied, "reservation_id outside the caller's preview namespace")
		}
	}
	if req.GetReservationId() == "" {
		return nil, status.Error(codes.InvalidArgument, "reservation_id is required")
	}
	res, err := h.svc.ValidateAndReserve(ctx, req.GetReservationId(), req.GetCode(), buyerID, req.GetCartSubtotal(), req.GetSellerId())
	if err != nil {
		return nil, internalError(ctx, h.logger, "validate and reserve", err)
	}
	return &promotionv1.ValidateAndReserveResponse{
		Valid:          res.Valid,
		Reason:         res.Reason,
		DiscountAmount: res.DiscountAmount,
		VoucherId:      res.VoucherID,
	}, nil
}

// CommitReservation is a saga RPC: only the order service's own service principal
// (scope promotion.reserve) may count a redemption against a voucher's quota.
func (h *VoucherHandler) CommitReservation(ctx context.Context, req *promotionv1.CommitReservationRequest) (*promotionv1.CommitReservationResponse, error) {
	if _, err := interceptor.RequireService(ctx, interceptor.ScopePromoReserve); err != nil {
		return nil, err
	}
	if req.GetReservationId() == "" {
		return nil, status.Error(codes.InvalidArgument, "reservation_id is required")
	}
	committed, err := h.svc.CommitReservation(ctx, req.GetReservationId())
	if err != nil {
		return nil, internalError(ctx, h.logger, "commit reservation", err)
	}
	return &promotionv1.CommitReservationResponse{Committed: committed}, nil
}

// ReleaseReservation is a saga RPC: service-only, like CommitReservation.
func (h *VoucherHandler) ReleaseReservation(ctx context.Context, req *promotionv1.ReleaseReservationRequest) (*promotionv1.ReleaseReservationResponse, error) {
	if _, err := interceptor.RequireService(ctx, interceptor.ScopePromoReserve); err != nil {
		return nil, err
	}
	if req.GetReservationId() == "" {
		return nil, status.Error(codes.InvalidArgument, "reservation_id is required")
	}
	released, err := h.svc.ReleaseReservation(ctx, req.GetReservationId())
	if err != nil {
		return nil, internalError(ctx, h.logger, "release reservation", err)
	}
	return &promotionv1.ReleaseReservationResponse{Released: released}, nil
}
