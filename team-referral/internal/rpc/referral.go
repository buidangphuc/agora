package rpc

import (
	"context"
	"errors"
	"log/slog"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/grpcx"
	commonv1 "github.com/buidangphuc/team-referral/generated/platform/common/v1"
	referralv1 "github.com/buidangphuc/team-referral/generated/platform/referral/v1"
	"github.com/buidangphuc/team-referral/internal/referral"
)

type Handler struct {
	referralv1.UnimplementedReferralServiceServer
	svc    *referral.Service
	logger *slog.Logger
	errs   grpcx.ErrorTable
}

func NewHandler(svc *referral.Service, logger *slog.Logger) (*Handler, error) {
	if svc == nil {
		return nil, errors.New("rpc: service is required")
	}
	if logger == nil {
		return nil, errors.New("rpc: logger is required")
	}
	errs, err := grpcx.NewErrorTable(
		grpcx.Rule{Target: referral.ErrEmptyUser, Code: codes.Unauthenticated},
		grpcx.Rule{Target: referral.ErrEmptyCode, Code: codes.InvalidArgument},
		grpcx.Rule{Target: referral.ErrCodeNotFound, Code: codes.NotFound},
		grpcx.Rule{Target: referral.ErrSelfReferral, Code: codes.FailedPrecondition},
		grpcx.Rule{Target: referral.ErrAlreadyRedeemed, Code: codes.FailedPrecondition},
	)
	if err != nil {
		return nil, err
	}
	return &Handler{svc: svc, logger: logger, errs: errs}, nil
}

func Register(srv *grpc.Server, h *Handler) {
	referralv1.RegisterReferralServiceServer(srv, h)
}

func (h *Handler) fail(ctx context.Context, err error) error {
	return grpcx.Status(ctx, h.logger, err, h.errs)
}

func (h *Handler) CreateReferralCode(ctx context.Context, _ *referralv1.CreateReferralCodeRequest) (*referralv1.CreateReferralCodeResponse, error) {
	p, err := auth.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	code, err := h.svc.CreateCode(ctx, p.ID)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	return &referralv1.CreateReferralCodeResponse{Code: code}, nil
}

func (h *Handler) GetMyReferral(ctx context.Context, _ *referralv1.GetMyReferralRequest) (*referralv1.GetMyReferralResponse, error) {
	p, err := auth.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	mr, err := h.svc.GetMyReferral(ctx, p.ID)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	return &referralv1.GetMyReferralResponse{
		Code:         mr.Code,
		InvitedCount: mr.InvitedCount,
		RewardsTotal: mr.RewardsTotal,
	}, nil
}

func (h *Handler) RedeemReferral(ctx context.Context, req *referralv1.RedeemReferralRequest) (*referralv1.RedeemReferralResponse, error) {
	p, err := auth.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.svc.Redeem(ctx, p.ID, req.GetCode()); err != nil {
		return nil, h.fail(ctx, err)
	}
	return &referralv1.RedeemReferralResponse{}, nil
}

func (h *Handler) ListReferralRewards(ctx context.Context, req *referralv1.ListReferralRewardsRequest) (*referralv1.ListReferralRewardsResponse, error) {
	p, err := auth.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	rewards, nextCursor, err := h.svc.ListRewards(ctx, p.ID, req)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	return &referralv1.ListReferralRewardsResponse{
		Rewards: rewards,
		Page:    &commonv1.PageResponse{NextCursor: nextCursor, Total: int64(len(rewards))},
	}, nil
}
