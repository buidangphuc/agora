package handler

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	verificationv1 "github.com/buidangphuc/team-verification/generated/platform/verification/v1"
	"github.com/buidangphuc/team-verification/internal/interceptor"
	"github.com/buidangphuc/team-verification/internal/repository"
	"github.com/buidangphuc/team-verification/internal/service"
)

// VerificationHandler adapts the gRPC VerificationService to the use-case layer.
type VerificationHandler struct {
	verificationv1.UnimplementedVerificationServiceServer
	svc *service.VerificationService
}

func NewVerificationHandler(svc *service.VerificationService) *VerificationHandler {
	return &VerificationHandler{svc: svc}
}

// SubmitKyc submits a KYC document reference for the authenticated caller. The
// user id comes from the forwarded principal (auth-scoped, never from the body).
func (h *VerificationHandler) SubmitKyc(ctx context.Context, req *verificationv1.SubmitKycRequest) (*verificationv1.SubmitKycResponse, error) {
	userID := interceptor.UserIDOrDemo(ctx)
	sub, err := h.svc.Submit(ctx, userID, req.GetDocType(), req.GetDocRef())
	if err != nil {
		return nil, mapErr(err)
	}
	return &verificationv1.SubmitKycResponse{
		Id:     sub.ID,
		Status: toProtoStatus(sub.Status),
	}, nil
}

// GetVerificationStatus reports a user's status and badge eligibility. When a
// user_id is supplied it is honored (admin/lookup); otherwise the caller's own
// forwarded principal is used, so a user can always read their own status.
func (h *VerificationHandler) GetVerificationStatus(ctx context.Context, req *verificationv1.GetVerificationStatusRequest) (*verificationv1.GetVerificationStatusResponse, error) {
	userID := req.GetUserId()
	if userID == "" {
		userID = interceptor.UserIDOrDemo(ctx)
	}
	st, badge, err := h.svc.GetStatus(ctx, userID)
	if err != nil {
		return nil, mapErr(err)
	}
	return &verificationv1.GetVerificationStatusResponse{
		Status: toProtoStatus(st),
		Badge:  badge,
	}, nil
}

// ReviewKyc applies a reviewer's approve/reject decision (mock admin action).
func (h *VerificationHandler) ReviewKyc(ctx context.Context, req *verificationv1.ReviewKycRequest) (*verificationv1.ReviewKycResponse, error) {
	st, err := h.svc.Review(ctx, req.GetId(), req.GetDecision())
	if err != nil {
		return nil, mapErr(err)
	}
	return &verificationv1.ReviewKycResponse{Status: toProtoStatus(st)}, nil
}

// mapErr turns a service/repository error into the right gRPC status.
func mapErr(err error) error {
	switch {
	case errors.Is(err, service.ErrEmptyUser),
		errors.Is(err, service.ErrEmptyDocType),
		errors.Is(err, service.ErrEmptyDocRef),
		errors.Is(err, service.ErrEmptyID),
		errors.Is(err, service.ErrInvalidReview):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, repository.ErrNotFound):
		return status.Error(codes.NotFound, err.Error())
	default:
		return status.Errorf(codes.Internal, "%v", err)
	}
}

// toProtoStatus maps the domain status string to the proto enum.
func toProtoStatus(s repository.Status) verificationv1.VerificationStatus {
	switch s {
	case repository.StatusVerified:
		return verificationv1.VerificationStatus_VERIFICATION_STATUS_VERIFIED
	case repository.StatusRejected:
		return verificationv1.VerificationStatus_VERIFICATION_STATUS_REJECTED
	default:
		return verificationv1.VerificationStatus_VERIFICATION_STATUS_PENDING
	}
}
