package handler

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	commonv1 "github.com/buidangphuc/team-verification/generated/platform/common/v1"
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

const adminScope = "admin"

// SubmitKyc submits a KYC document reference for the authenticated end user. The
// owner is the forwarded principal id (never the body); anonymous callers are
// Unauthenticated and service principals PermissionDenied.
func (h *VerificationHandler) SubmitKyc(ctx context.Context, req *verificationv1.SubmitKycRequest) (*verificationv1.SubmitKycResponse, error) {
	p, err := interceptor.UserPrincipal(ctx)
	if err != nil {
		return nil, err
	}
	sub, err := h.svc.Submit(ctx, p.GetId(), req.GetDocType(), req.GetDocRef())
	if err != nil {
		return nil, mapErr(err)
	}
	return &verificationv1.SubmitKycResponse{
		Id:     sub.ID,
		Status: toProtoStatus(sub.Status),
	}, nil
}

// GetVerificationStatus reports a user's status and badge eligibility. An empty
// user_id (or the caller's own id) returns the caller's own status; any other
// user_id requires the admin scope.
func (h *VerificationHandler) GetVerificationStatus(ctx context.Context, req *verificationv1.GetVerificationStatusRequest) (*verificationv1.GetVerificationStatusResponse, error) {
	p, err := interceptor.AuthenticatedPrincipal(ctx)
	if err != nil {
		return nil, err
	}
	userID := req.GetUserId()
	switch {
	case userID == "" || userID == p.GetId():
		// Own status is only meaningful for an end user.
		if p.GetType() != commonv1.PrincipalType_PRINCIPAL_TYPE_USER {
			return nil, status.Error(codes.PermissionDenied, "user principal required")
		}
		userID = p.GetId()
	default:
		if err := interceptor.RequireScopes(ctx, adminScope); err != nil {
			return nil, err
		}
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

// ReviewKyc applies an approve/reject decision. Admin scope only, and a reviewer
// may never review their own submission.
func (h *VerificationHandler) ReviewKyc(ctx context.Context, req *verificationv1.ReviewKycRequest) (*verificationv1.ReviewKycResponse, error) {
	p, err := interceptor.AuthenticatedPrincipal(ctx)
	if err != nil {
		return nil, err
	}
	if err := interceptor.RequireScopes(ctx, adminScope); err != nil {
		return nil, err
	}
	sub, err := h.svc.Get(ctx, req.GetId())
	if err != nil {
		return nil, mapErr(err)
	}
	if sub.UserID == p.GetId() {
		return nil, status.Error(codes.PermissionDenied, "reviewers cannot review their own submission")
	}
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
