// Package rpc is the transport layer: it decodes requests, reads the caller,
// calls the referral domain and maps its errors to gRPC status. It holds no
// business logic.
package rpc

import (
	"context"
	"log/slog"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/auth"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/grpcx"

	commonv1 "github.com/buidangphuc/team-referral/generated/platform/common/v1"
	referralv1 "github.com/buidangphuc/team-referral/generated/platform/referral/v1"
	"github.com/buidangphuc/team-referral/internal/referral"
)

// Handler implements platform.referral.v1.ReferralService. Every RPC is
// user-owned, so it resolves the caller from the principal (never from the
// request body) and rejects anonymous callers.
type Handler struct {
	referralv1.UnimplementedReferralServiceServer
	svc    *referral.Service
	logger *slog.Logger
	errs   grpcx.ErrorTable
}

// NewHandler returns a Handler that maps domain errors with its error table.
func NewHandler(svc *referral.Service, logger *slog.Logger) *Handler {
	return &Handler{
		svc:    svc,
		logger: logger,
		errs: grpcx.ErrorTable{
			referral.ErrEmptyUser:       codes.Unauthenticated,
			referral.ErrEmptyCode:       codes.InvalidArgument,
			referral.ErrCodeNotFound:    codes.NotFound,
			referral.ErrSelfReferral:    codes.FailedPrecondition,
			referral.ErrAlreadyRedeemed: codes.FailedPrecondition,
		},
	}
}

// Register adds the referral service to s.
func Register(s grpc.ServiceRegistrar, h *Handler) {
	referralv1.RegisterReferralServiceServer(s, h)
}

// mapErr turns a domain or store error into the right gRPC status; unknown
// errors are logged and become Internal with a fixed message.
func (h *Handler) mapErr(ctx context.Context, err error) error {
	return grpcx.Status(ctx, h.logger, err, h.errs)
}

// CreateReferralCode returns the caller's code, minting one if needed.
func (h *Handler) CreateReferralCode(ctx context.Context, _ *referralv1.CreateReferralCodeRequest) (*referralv1.CreateReferralCodeResponse, error) {
	p, err := auth.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	code, err := h.svc.CreateCode(ctx, p.ID)
	if err != nil {
		return nil, h.mapErr(ctx, err)
	}
	return &referralv1.CreateReferralCodeResponse{Code: code}, nil
}

// GetMyReferral returns the caller's code, invited count and reward total.
func (h *Handler) GetMyReferral(ctx context.Context, _ *referralv1.GetMyReferralRequest) (*referralv1.GetMyReferralResponse, error) {
	p, err := auth.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	mr, err := h.svc.GetMyReferral(ctx, p.ID)
	if err != nil {
		return nil, h.mapErr(ctx, err)
	}
	return &referralv1.GetMyReferralResponse{
		Code:         mr.Code,
		InvitedCount: mr.InvitedCount,
		RewardsTotal: mr.RewardsTotal,
	}, nil
}

// RedeemReferral attributes the caller to the owner of req.Code.
func (h *Handler) RedeemReferral(ctx context.Context, req *referralv1.RedeemReferralRequest) (*referralv1.RedeemReferralResponse, error) {
	p, err := auth.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.svc.Redeem(ctx, p.ID, req.GetCode()); err != nil {
		return nil, h.mapErr(ctx, err)
	}
	return &referralv1.RedeemReferralResponse{}, nil
}

// ListReferralRewards returns a page of the caller's reward ledger.
func (h *Handler) ListReferralRewards(ctx context.Context, req *referralv1.ListReferralRewardsRequest) (*referralv1.ListReferralRewardsResponse, error) {
	p, err := auth.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	rewards, nextCursor, err := h.svc.ListRewards(ctx, p.ID, req)
	if err != nil {
		return nil, h.mapErr(ctx, err)
	}
	return &referralv1.ListReferralRewardsResponse{
		Rewards: rewards,
		Page:    &commonv1.PageResponse{NextCursor: nextCursor, Total: int64(len(rewards))},
	}, nil
}
