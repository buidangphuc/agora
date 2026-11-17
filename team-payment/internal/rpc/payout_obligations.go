package rpc

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	paymentv1 "github.com/buidangphuc/team-payment/generated/platform/payment/v1"
)

const internalReadScope = "payment.internal.read"

func (h *Handler) GetPayoutObligations(ctx context.Context, req *paymentv1.GetPayoutObligationsRequest) (*paymentv1.GetPayoutObligationsResponse, error) {
	if _, err := auth.RequireService(ctx, internalReadScope); err != nil {
		return nil, err
	}
	if req.GetUserId() == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}
	o, err := h.svc.GetPayoutObligations(ctx, req.GetUserId())
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	return &paymentv1.GetPayoutObligationsResponse{
		AvailableBalance: o.Available,
		HeldBalance:      o.Held,
		PendingPayouts:   o.Pending,
	}, nil
}
