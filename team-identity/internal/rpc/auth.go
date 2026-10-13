// Package rpc is the gRPC transport for team-identity: handlers for the auth,
// address, session and public-profile services. Domain errors become gRPC status
// here and nowhere else.
package rpc

import (
	"context"
	"log/slog"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/auth"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/grpcx"

	commonv1 "github.com/buidangphuc/team-identity/generated/platform/common/v1"
	identityv1 "github.com/buidangphuc/team-identity/generated/platform/identity/v1"

	"github.com/buidangphuc/team-identity/internal/identity"
)

// AuthHandler implements identityv1.AuthServiceServer.
type AuthHandler struct {
	identityv1.UnimplementedAuthServiceServer
	svc *identity.AuthService

	exposeResetToken bool // dev/e2e only: return the raw reset token in the response
	logger           *slog.Logger
}

// WithLogger sets the logger used for the password_reset.issued audit line.
func (h *AuthHandler) WithLogger(l *slog.Logger) *AuthHandler {
	if l != nil {
		h.logger = l
	}
	return h
}

// WithExposeResetToken makes RequestPasswordReset return the raw reset token.
// Off by default: the token is a bearer credential for the account.
func (h *AuthHandler) WithExposeResetToken(on bool) *AuthHandler {
	h.exposeResetToken = on
	return h
}

// NewAuthHandler builds the handler over the identity service.
func NewAuthHandler(svc *identity.AuthService) *AuthHandler {
	return &AuthHandler{svc: svc, logger: slog.Default()}
}

// Register creates a user, opens a session and returns a token.
func (h *AuthHandler) Register(
	ctx context.Context,
	req *identityv1.RegisterRequest,
) (*identityv1.RegisterResponse, error) {
	res, err := h.svc.Register(withClient(ctx), req.GetUsername(), req.GetPassword(), req.GetRole())
	if err != nil {
		return nil, grpcx.Status(ctx, h.logger, err, authErrors)
	}
	return &identityv1.RegisterResponse{Result: toResult(res)}, nil
}

// Login checks the credentials, opens a session and returns a token.
func (h *AuthHandler) Login(
	ctx context.Context,
	req *identityv1.LoginRequest,
) (*identityv1.LoginResponse, error) {
	res, err := h.svc.Login(withClient(ctx), req.GetUsername(), req.GetPassword())
	if err != nil {
		return nil, grpcx.Status(ctx, h.logger, err, authErrors)
	}
	return &identityv1.LoginResponse{Result: toResult(res)}, nil
}

// ChangePassword changes the authenticated user's own password.
func (h *AuthHandler) ChangePassword(
	ctx context.Context,
	req *identityv1.ChangePasswordRequest,
) (*identityv1.ChangePasswordResponse, error) {
	// The target is always the authenticated user: req.user_id is ignored so a
	// caller can never change another account's password. Service/anonymous
	// principals have no password to change.
	principal, err := auth.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	if principal.Type != auth.TypeUser {
		return nil, status.Error(codes.PermissionDenied, "user principal required")
	}
	userID := principal.ID
	if err := h.svc.ChangePassword(ctx, userID, req.GetOldPassword(), req.GetNewPassword()); err != nil {
		return nil, grpcx.Status(ctx, h.logger, err, authErrors)
	}
	return &identityv1.ChangePasswordResponse{Success: true}, nil
}

// RequestPasswordReset issues a reset token and returns its expiry (and the token only when exposing is enabled).
func (h *AuthHandler) RequestPasswordReset(
	ctx context.Context,
	req *identityv1.RequestPasswordResetRequest,
) (*identityv1.RequestPasswordResetResponse, error) {
	iss, err := h.svc.IssuePasswordReset(ctx, req.GetUsername())
	if err != nil {
		return nil, grpcx.Status(ctx, h.logger, err, authErrors)
	}
	// Audit line: never the raw token, a token in logs is the same leak by another route.
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

// ResetPassword sets a new password from a valid, unused reset token.
func (h *AuthHandler) ResetPassword(
	ctx context.Context,
	req *identityv1.ResetPasswordRequest,
) (*identityv1.ResetPasswordResponse, error) {
	if err := h.svc.ResetPassword(ctx, req.GetToken(), req.GetNewPassword()); err != nil {
		return nil, grpcx.Status(ctx, h.logger, err, authErrors)
	}
	return &identityv1.ResetPasswordResponse{Success: true}, nil
}

// withClient copies the gateway's audit-only client context into ctx.
func withClient(ctx context.Context) context.Context {
	return identity.WithClient(ctx, clientFromContext(ctx))
}

func toResult(r identity.AuthResult) *identityv1.AuthResult {
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
