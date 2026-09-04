package handler

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	commonv1 "github.com/buidangphuc/team-referral/generated/platform/common/v1"
	referralv1 "github.com/buidangphuc/team-referral/generated/platform/referral/v1"
	"github.com/buidangphuc/team-referral/internal/interceptor"
	"github.com/buidangphuc/team-referral/internal/service"
)

// ReferralHandler adapts the gRPC ReferralService contract onto the application
// service. Every RPC is user-owned, so the handler resolves the caller from the
// principal (never from the request body) and rejects anonymous callers.
type ReferralHandler struct {
	referralv1.UnimplementedReferralServiceServer
	svc *service.ReferralService
}

func NewReferralHandler(svc *service.ReferralService) *ReferralHandler {
	return &ReferralHandler{svc: svc}
}

// callerID returns the resolved user id, or an Unauthenticated error when the
// request carried no verified principal.
func (h *ReferralHandler) callerID(ctx context.Context) (string, error) {
	p := interceptor.FromContext(ctx)
	if p.Anonymous || p.UserID == "" {
		return "", status.Error(codes.Unauthenticated, "authentication required")
	}
	return p.UserID, nil
}

// mapErr turns a service error into the right gRPC status.
func mapErr(err error) error {
	switch {
	case errors.Is(err, service.ErrEmptyUser):
		return status.Error(codes.Unauthenticated, err.Error())
	case errors.Is(err, service.ErrEmptyCode):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, service.ErrCodeNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, service.ErrSelfReferral),
		errors.Is(err, service.ErrAlreadyRedeemed):
		return status.Error(codes.FailedPrecondition, err.Error())
	default:
		return status.Errorf(codes.Internal, "%v", err)
	}
}

func (h *ReferralHandler) CreateReferralCode(ctx context.Context, _ *referralv1.CreateReferralCodeRequest) (*referralv1.CreateReferralCodeResponse, error) {
	userID, err := h.callerID(ctx)
	if err != nil {
		return nil, err
	}
	code, err := h.svc.CreateCode(ctx, userID)
	if err != nil {
		return nil, mapErr(err)
	}
	return &referralv1.CreateReferralCodeResponse{Code: code}, nil
}

func (h *ReferralHandler) GetMyReferral(ctx context.Context, _ *referralv1.GetMyReferralRequest) (*referralv1.GetMyReferralResponse, error) {
	userID, err := h.callerID(ctx)
	if err != nil {
		return nil, err
	}
	mr, err := h.svc.GetMyReferral(ctx, userID)
	if err != nil {
		return nil, mapErr(err)
	}
	return &referralv1.GetMyReferralResponse{
		Code:         mr.Code,
		InvitedCount: mr.InvitedCount,
		RewardsTotal: mr.RewardsTotal,
	}, nil
}

func (h *ReferralHandler) RedeemReferral(ctx context.Context, req *referralv1.RedeemReferralRequest) (*referralv1.RedeemReferralResponse, error) {
	userID, err := h.callerID(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.svc.Redeem(ctx, userID, req.GetCode()); err != nil {
		return nil, mapErr(err)
	}
	return &referralv1.RedeemReferralResponse{}, nil
}

func (h *ReferralHandler) ListReferralRewards(ctx context.Context, req *referralv1.ListReferralRewardsRequest) (*referralv1.ListReferralRewardsResponse, error) {
	userID, err := h.callerID(ctx)
	if err != nil {
		return nil, err
	}
	rewards, nextCursor, err := h.svc.ListRewards(ctx, userID, req)
	if err != nil {
		return nil, mapErr(err)
	}
	return &referralv1.ListReferralRewardsResponse{
		Rewards: rewards,
		Page:    &commonv1.PageResponse{NextCursor: nextCursor, Total: int64(len(rewards))},
	}, nil
}
