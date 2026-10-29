package rpc

import (
	"context"
	"log/slog"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	commonv1 "github.com/buidangphuc/team-identity/generated/platform/common/v1"
	identityv1 "github.com/buidangphuc/team-identity/generated/platform/identity/v1"
	"github.com/buidangphuc/team-identity/internal/identity"
)

const maxRefreshTokenLen = 256

type AuthHandler struct {
	identityv1.UnimplementedAuthServiceServer
	svc *identity.AuthService

	exposeResetToken bool
	logger           *slog.Logger
}

func NewAuthHandler(svc *identity.AuthService) *AuthHandler {
	return &AuthHandler{svc: svc, logger: slog.Default()}
}

func (h *AuthHandler) WithLogger(l *slog.Logger) *AuthHandler {
	if l != nil {
		h.logger = l
	}
	return h
}

func (h *AuthHandler) WithExposeResetToken(on bool) *AuthHandler {
	h.exposeResetToken = on
	return h
}

func (h *AuthHandler) Register(ctx context.Context, req *identityv1.RegisterRequest) (*identityv1.RegisterResponse, error) {
	res, err := h.svc.Register(withClient(ctx), req.GetUsername(), req.GetPassword(), req.GetRole())
	if err != nil {
		return nil, statusErr(ctx, h.logger, err)
	}
	return &identityv1.RegisterResponse{Result: toResult(res)}, nil
}

func (h *AuthHandler) Login(ctx context.Context, req *identityv1.LoginRequest) (*identityv1.LoginResponse, error) {
	res, err := h.svc.Login(withClient(ctx), req.GetUsername(), req.GetPassword())
	if err != nil {
		return nil, statusErr(ctx, h.logger, err)
	}
	return &identityv1.LoginResponse{Result: toResult(res)}, nil
}

func (h *AuthHandler) RefreshSession(ctx context.Context, req *identityv1.RefreshSessionRequest) (*identityv1.RefreshSessionResponse, error) {
	tok := req.GetRefreshToken()
	if tok == "" || len(tok) > maxRefreshTokenLen {
		return nil, statusErr(ctx, h.logger, identity.ErrInvalidRefreshToken)
	}
	res, err := h.svc.RefreshSession(withClient(ctx), tok)
	if err != nil {
		return nil, statusErr(ctx, h.logger, err)
	}
	return &identityv1.RefreshSessionResponse{Result: toResult(res)}, nil
}

func (h *AuthHandler) ChangePassword(ctx context.Context, req *identityv1.ChangePasswordRequest) (*identityv1.ChangePasswordResponse, error) {
	p, err := auth.RequireUser(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.svc.ChangePassword(ctx, p.ID, req.GetOldPassword(), req.GetNewPassword()); err != nil {
		return nil, statusErr(ctx, h.logger, err)
	}
	return &identityv1.ChangePasswordResponse{Success: true}, nil
}

func (h *AuthHandler) RequestPasswordReset(ctx context.Context, req *identityv1.RequestPasswordResetRequest) (*identityv1.RequestPasswordResetResponse, error) {
	iss, err := h.svc.IssuePasswordReset(ctx, req.GetUsername())
	if err != nil {
		return nil, statusErr(ctx, h.logger, err)
	}
	h.logger.InfoContext(ctx, "password_reset.issued",
		"user_id", iss.UserID,
		"expires_at", iss.ExpiresAt.Unix(),
		"token_fingerprint", identity.TokenFingerprint(iss.Token))
	resp := &identityv1.RequestPasswordResetResponse{ExpiresAt: iss.ExpiresAt.Unix()}
	if h.exposeResetToken {
		resp.ResetToken = iss.Token
	}
	return resp, nil
}

func (h *AuthHandler) ResetPassword(ctx context.Context, req *identityv1.ResetPasswordRequest) (*identityv1.ResetPasswordResponse, error) {
	if err := h.svc.ResetPassword(ctx, req.GetToken(), req.GetNewPassword()); err != nil {
		return nil, statusErr(ctx, h.logger, err)
	}
	return &identityv1.ResetPasswordResponse{Success: true}, nil
}

func toResult(r identity.AuthResult) *identityv1.AuthResult {
	var refreshExp *timestamppb.Timestamp
	if !r.RefreshExpiresAt.IsZero() {
		refreshExp = timestamppb.New(r.RefreshExpiresAt)
	}
	return &identityv1.AuthResult{
		Token:            r.Token,
		RefreshToken:     r.RefreshToken,
		RefreshExpiresAt: refreshExp,
		Username:         r.Username,
		Principal: &commonv1.Principal{
			Id:     r.UserID,
			Type:   commonv1.PrincipalType_PRINCIPAL_TYPE_USER,
			Scopes: r.Scopes,
		},
	}
}
