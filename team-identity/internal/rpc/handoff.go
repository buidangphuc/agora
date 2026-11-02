package rpc

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"

	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/timestamppb"

	identityv1 "github.com/buidangphuc/team-identity/generated/platform/identity/v1"
)

func (h *AuthHandler) CreateHandoffCode(ctx context.Context, req *identityv1.CreateHandoffCodeRequest) (*identityv1.CreateHandoffCodeResponse, error) {
	res, err := h.svc.Mint(ctx, callerSID(ctx), req.GetTargetApp())
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
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	for _, v := range md.Get("authorization") {
		raw, found := strings.CutPrefix(v, "Bearer ")
		if !found {
			continue
		}
		parts := strings.Split(raw, ".")
		if len(parts) != 3 {
			continue
		}
		payload, err := base64.RawURLEncoding.DecodeString(parts[1])
		if err != nil {
			continue
		}
		var c struct {
			SID string `json:"sid"`
		}
		if json.Unmarshal(payload, &c) == nil && c.SID != "" {
			return c.SID
		}
	}
	return ""
}
