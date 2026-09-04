// Package handler is the gRPC transport adapter for AuthService.
package handler

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	commonv1 "github.com/buidangphuc/team-identity/generated/platform/common/v1"
	identityv1 "github.com/buidangphuc/team-identity/generated/platform/identity/v1"

	"github.com/buidangphuc/team-identity/internal/interceptor"
	"github.com/buidangphuc/team-identity/internal/repository"
	"github.com/buidangphuc/team-identity/internal/service"
)

// AuthHandler implements identityv1.AuthServiceServer.
type AuthHandler struct {
	identityv1.UnimplementedAuthServiceServer
	svc *service.AuthService
}

func NewAuthHandler(svc *service.AuthService) *AuthHandler {
	return &AuthHandler{svc: svc}
}

func (h *AuthHandler) Register(
	ctx context.Context,
	req *identityv1.RegisterRequest,
) (*identityv1.RegisterResponse, error) {
	res, err := h.svc.Register(ctx, req.GetUsername(), req.GetPassword(), req.GetRole())
	if err != nil {
		return nil, mapErr(err)
	}
	return &identityv1.RegisterResponse{Result: toResult(res)}, nil
}

func (h *AuthHandler) Login(
	ctx context.Context,
	req *identityv1.LoginRequest,
) (*identityv1.LoginResponse, error) {
	res, err := h.svc.Login(ctx, req.GetUsername(), req.GetPassword())
	if err != nil {
		return nil, mapErr(err)
	}
	return &identityv1.LoginResponse{Result: toResult(res)}, nil
}

func (h *AuthHandler) ChangePassword(
	ctx context.Context,
	req *identityv1.ChangePasswordRequest,
) (*identityv1.ChangePasswordResponse, error) {
	userID := req.GetUserId()
	if userID == "" {
		if p, ok := interceptor.PrincipalFromContext(ctx); ok && p != nil {
			userID = p.GetId()
		}
	}
	if err := h.svc.ChangePassword(ctx, userID, req.GetOldPassword(), req.GetNewPassword()); err != nil {
		return nil, mapErr(err)
	}
	return &identityv1.ChangePasswordResponse{Success: true}, nil
}

func (h *AuthHandler) RequestPasswordReset(
	ctx context.Context,
	req *identityv1.RequestPasswordResetRequest,
) (*identityv1.RequestPasswordResetResponse, error) {
	token, expiresAt, err := h.svc.RequestPasswordReset(ctx, req.GetUsername())
	if err != nil {
		return nil, mapErr(err)
	}
	return &identityv1.RequestPasswordResetResponse{
		ResetToken: token,
		ExpiresAt:  expiresAt.Unix(),
	}, nil
}

func (h *AuthHandler) ResetPassword(
	ctx context.Context,
	req *identityv1.ResetPasswordRequest,
) (*identityv1.ResetPasswordResponse, error) {
	if err := h.svc.ResetPassword(ctx, req.GetToken(), req.GetNewPassword()); err != nil {
		return nil, mapErr(err)
	}
	return &identityv1.ResetPasswordResponse{Success: true}, nil
}

func toResult(r service.AuthResult) *identityv1.AuthResult {
	return &identityv1.AuthResult{
		Token:    r.Token,
		Username: r.Username,
		Principal: &commonv1.Principal{
			Id:     r.UserID,
			Type:   commonv1.PrincipalType_PRINCIPAL_TYPE_USER,
			Scopes: r.Scopes,
		},
	}
}

func mapErr(err error) error {
	switch {
	case errors.Is(err, service.ErrInvalidInput):
		return status.Error(codes.InvalidArgument, "invalid input")
	case errors.Is(err, service.ErrInvalidCredentials):
		return status.Error(codes.Unauthenticated, "invalid credentials")
	case errors.Is(err, service.ErrInvalidToken):
		return status.Error(codes.InvalidArgument, "invalid reset token")
	case errors.Is(err, service.ErrTokenExpired):
		return status.Error(codes.InvalidArgument, "reset token expired")
	case errors.Is(err, service.ErrTokenAlreadyUsed):
		return status.Error(codes.InvalidArgument, "reset token already used")
	case errors.Is(err, repository.ErrConflict):
		return status.Error(codes.AlreadyExists, "username already exists")
	case errors.Is(err, repository.ErrNotFound):
		return status.Error(codes.NotFound, "user not found")
	case errors.Is(err, repository.ErrTokenNotFound):
		return status.Error(codes.NotFound, "reset token not found")
	default:
		return status.Error(codes.Internal, "internal error")
	}
}
