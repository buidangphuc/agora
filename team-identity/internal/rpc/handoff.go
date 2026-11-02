package rpc

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	identityv1 "github.com/buidangphuc/team-identity/generated/platform/identity/v1"
)

const mdPrincipalSessionID = "x-principal-sid"

func (h *AuthHandler) CreateHandoffCode(ctx context.Context, req *identityv1.CreateHandoffCodeRequest) (*identityv1.CreateHandoffCodeResponse, error) {
	if _, err := auth.RequireUser(ctx); err != nil {
		return nil, err
	}
	sid := callerSID(ctx)
	if sid == "" {
		return nil, status.Error(codes.Unauthenticated, "authentication required")
	}
	res, err := h.svc.Mint(ctx, sid, req.GetTargetApp())
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

func callerSID(ctx context.Context) string {
	md, _ := metadata.FromIncomingContext(ctx)
	if v := md.Get(mdPrincipalSessionID); len(v) > 0 {
		return v[0]
	}
	return ""
}
