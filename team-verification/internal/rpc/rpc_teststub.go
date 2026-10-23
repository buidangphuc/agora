package rpc

import (
	"context"
	"log/slog"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auditx"
	verificationv1 "github.com/buidangphuc/team-verification/generated/platform/verification/v1"
	"github.com/buidangphuc/team-verification/internal/verification"
)

type Handler struct {
	verificationv1.UnimplementedVerificationServiceServer
}

func NewHandler(_ *verification.VerificationService, _ auditx.Writer, _ *slog.Logger) (*Handler, error) {
	return &Handler{}, nil
}

func Register(srv *grpc.Server, h *Handler) {
	verificationv1.RegisterVerificationServiceServer(srv, h)
}

func (*Handler) SubmitKyc(context.Context, *verificationv1.SubmitKycRequest) (*verificationv1.SubmitKycResponse, error) {
	return nil, status.Error(codes.Unimplemented, "stub")
}

func (*Handler) GetVerificationStatus(context.Context, *verificationv1.GetVerificationStatusRequest) (*verificationv1.GetVerificationStatusResponse, error) {
	return nil, status.Error(codes.Unimplemented, "stub")
}

func (*Handler) ReviewKyc(context.Context, *verificationv1.ReviewKycRequest) (*verificationv1.ReviewKycResponse, error) {
	return nil, status.Error(codes.Unimplemented, "stub")
}
