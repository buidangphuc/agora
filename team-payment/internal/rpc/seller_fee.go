package rpc

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	paymentv1 "github.com/buidangphuc/team-payment/generated/platform/payment/v1"
	"github.com/buidangphuc/team-payment/internal/store"
)

func (h *Handler) ChargeSellerFee(ctx context.Context, req *paymentv1.ChargeSellerFeeRequest) (*paymentv1.ChargeSellerFeeResponse, error) {
	p, err := auth.RequireScopes(ctx, "payment:write")
	if err != nil {
		return nil, err
	}
	if p.Type != auth.TypeService {
		return nil, status.Error(codes.PermissionDenied, "service callers only")
	}
	if req.GetSellerId() == "" || req.GetReferenceId() == "" {
		return nil, status.Error(codes.InvalidArgument, "seller_id and reference_id are required")
	}
	if req.GetAmount() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "amount must be positive")
	}
	if req.GetKind() != paymentv1.SellerFeeKind_SELLER_FEE_KIND_SUBSCRIPTION {
		return nil, status.Error(codes.InvalidArgument, "unsupported fee kind")
	}
	entry, created, err := h.svc.ChargeSellerFee(ctx, req.GetSellerId(), req.GetAmount(), req.GetReferenceId())
	if err != nil {
		if errors.Is(err, store.ErrReferenceRequired) {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		return nil, h.fail(ctx, err)
	}
	return &paymentv1.ChargeSellerFeeResponse{EntryId: entry.ID, Created: created}, nil
}
