// Package rpc is the transport layer: it decodes requests, reads the caller,
// calls the verification domain and maps its errors to gRPC status. It holds no
// business logic.
package rpc

import (
	"context"
	"log/slog"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/auth"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/grpcx"

	verificationv1 "github.com/buidangphuc/team-verification/generated/platform/verification/v1"
	"github.com/buidangphuc/team-verification/internal/store"
	"github.com/buidangphuc/team-verification/internal/verification"
)

const adminScope = "admin"

// Handler implements platform.verification.v1.VerificationService.
type Handler struct {
	verificationv1.UnimplementedVerificationServiceServer
	svc    *verification.Service
	logger *slog.Logger
	errs   grpcx.ErrorTable
}

// NewHandler returns a Handler that maps domain errors with its error table;
// the client sees each sentinel error's own text.
func NewHandler(svc *verification.Service, logger *slog.Logger) (*Handler, error) {
	errs, err := grpcx.NewErrorTable(
		grpcx.Rule{Target: verification.ErrEmptyUser, Code: codes.InvalidArgument},
		grpcx.Rule{Target: verification.ErrEmptyDocType, Code: codes.InvalidArgument},
		grpcx.Rule{Target: verification.ErrEmptyDocRef, Code: codes.InvalidArgument},
		grpcx.Rule{Target: verification.ErrEmptyID, Code: codes.InvalidArgument},
		grpcx.Rule{Target: verification.ErrInvalidReview, Code: codes.InvalidArgument},
		grpcx.Rule{Target: verification.ErrNotFound, Code: codes.NotFound},
		grpcx.Rule{Target: verification.ErrAlreadyReviewed, Code: codes.FailedPrecondition},
	)
	if err != nil {
		return nil, err
	}
	return &Handler{svc: svc, logger: logger, errs: errs}, nil
}

// Register adds the verification service to s.
func Register(s grpc.ServiceRegistrar, h *Handler) {
	verificationv1.RegisterVerificationServiceServer(s, h)
}

// mapErr turns a domain or store error into the right gRPC status; unknown
// errors are logged and become Internal with a fixed message.
func (h *Handler) mapErr(ctx context.Context, err error) error {
	return grpcx.Status(ctx, h.logger, err, h.errs)
}

// SubmitKyc submits a KYC document reference for the authenticated end user. The
// owner is the forwarded principal id (never the body); anonymous callers are
// Unauthenticated and service principals PermissionDenied.
func (h *Handler) SubmitKyc(ctx context.Context, req *verificationv1.SubmitKycRequest) (*verificationv1.SubmitKycResponse, error) {
	p, err := userPrincipal(ctx)
	if err != nil {
		return nil, err
	}
	sub, err := h.svc.Submit(ctx, p.ID, req.GetDocType(), req.GetDocRef())
	if err != nil {
		return nil, h.mapErr(ctx, err)
	}
	return &verificationv1.SubmitKycResponse{
		Id:     sub.ID,
		Status: toProtoStatus(sub.Status),
	}, nil
}

// GetVerificationStatus reports a user's status and badge eligibility. An empty
// user_id (or the caller's own id) returns the caller's own status; any other
// user_id requires the admin scope.
func (h *Handler) GetVerificationStatus(ctx context.Context, req *verificationv1.GetVerificationStatusRequest) (*verificationv1.GetVerificationStatusResponse, error) {
	p, err := authenticated(ctx)
	if err != nil {
		return nil, err
	}
	userID := req.GetUserId()
	switch userID {
	case "", p.ID:
		// Own status is only meaningful for an end user.
		if p.Type != auth.TypeUser {
			return nil, status.Error(codes.PermissionDenied, "user principal required")
		}
		userID = p.ID
	default:
		if _, err := auth.RequireScopes(ctx, adminScope); err != nil {
			return nil, err
		}
	}
	st, badge, err := h.svc.GetStatus(ctx, userID)
	if err != nil {
		return nil, h.mapErr(ctx, err)
	}
	return &verificationv1.GetVerificationStatusResponse{
		Status: toProtoStatus(st),
		Badge:  badge,
	}, nil
}

// ReviewKyc applies an approve/reject decision. Admin scope only, and a reviewer
// may never review their own submission.
func (h *Handler) ReviewKyc(ctx context.Context, req *verificationv1.ReviewKycRequest) (*verificationv1.ReviewKycResponse, error) {
	p, err := authenticated(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := auth.RequireScopes(ctx, adminScope); err != nil {
		return nil, err
	}
	sub, err := h.svc.Get(ctx, req.GetId())
	if err != nil {
		return nil, h.mapErr(ctx, err)
	}
	if sub.UserID == p.ID {
		return nil, status.Error(codes.PermissionDenied, "reviewers cannot review their own submission")
	}
	st, err := h.svc.Review(ctx, req.GetId(), req.GetDecision())
	if err != nil {
		return nil, h.mapErr(ctx, err)
	}
	return &verificationv1.ReviewKycResponse{Status: toProtoStatus(st)}, nil
}

// toProtoStatus maps the domain status string to the proto enum.
func toProtoStatus(s store.Status) verificationv1.VerificationStatus {
	switch s {
	case store.StatusVerified:
		return verificationv1.VerificationStatus_VERIFICATION_STATUS_VERIFIED
	case store.StatusRejected:
		return verificationv1.VerificationStatus_VERIFICATION_STATUS_REJECTED
	default:
		return verificationv1.VerificationStatus_VERIFICATION_STATUS_PENDING
	}
}
