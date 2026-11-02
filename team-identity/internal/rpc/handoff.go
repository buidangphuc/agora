package rpc

import (
	"context"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	identityv1 "github.com/buidangphuc/team-identity/generated/platform/identity/v1"
)

func (h *AuthHandler) CreateHandoffCode(ctx context.Context, req *identityv1.CreateHandoffCodeRequest) (*identityv1.CreateHandoffCodeResponse, error) {
	if _, err := auth.RequireUser(ctx); err != nil {
		return nil, err
	}
	res, err := h.svc.Mint(ctx, "", req.GetTargetApp())
	if err != nil {
		return nil, statusErr(ctx, h.logger, err)
	}
	return &identityv1.CreateHandoffCodeResponse{Code: res.Code, ExpiresAt: timestamppb.New(res.ExpiresAt)}, nil
}

func (h *AuthHandler) PeekHandoffCode(ctx context.Context, req *identityv1.PeekHandoffCodeRequest) (*identityv1.PeekHandoffCodeResponse, error) {
	res, err := h.svc.Peek(ctx, req.GetCode(), req.GetTargetApp())
	if err != nil {
		return nil, statusErr(ctx, h.logger, err)
	}
	return &identityv1.PeekHandoffCodeResponse{Username: res.Username, ExpiresAt: timestamppb.New(res.ExpiresAt)}, nil
}

func (h *AuthHandler) RedeemHandoffCode(ctx context.Context, req *identityv1.RedeemHandoffCodeRequest) (*identityv1.RedeemHandoffCodeResponse, error) {
	res, err := h.svc.Redeem(withClient(ctx), req.GetCode(), req.GetTargetApp())
	if err != nil {
		return nil, statusErr(ctx, h.logger, err)
	}
	return &identityv1.RedeemHandoffCodeResponse{Result: toResult(res)}, nil
}
