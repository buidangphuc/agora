package rpc

import (
	"context"
	"errors"
	"log/slog"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auditx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/grpcx"
	verificationv1 "github.com/buidangphuc/team-verification/generated/platform/verification/v1"
	"github.com/buidangphuc/team-verification/internal/store"
	"github.com/buidangphuc/team-verification/internal/verification"
)

const adminScope = "admin"

type Handler struct {
	verificationv1.UnimplementedVerificationServiceServer
	svc    *verification.VerificationService
	gate   *auditx.Gate
	logger *slog.Logger
	errs   grpcx.ErrorTable
}

func NewHandler(svc *verification.VerificationService, audit auditx.Writer, logger *slog.Logger) (*Handler, error) {
	if svc == nil {
		return nil, errors.New("rpc: service is required")
	}
	if logger == nil {
		return nil, errors.New("rpc: logger is required")
	}
	errs, err := grpcx.NewErrorTable(
		grpcx.Rule{Target: verification.ErrEmptyUser, Code: codes.InvalidArgument},
		grpcx.Rule{Target: verification.ErrEmptyDocType, Code: codes.InvalidArgument},
		grpcx.Rule{Target: verification.ErrEmptyDocRef, Code: codes.InvalidArgument},
		grpcx.Rule{Target: verification.ErrEmptyID, Code: codes.InvalidArgument},
		grpcx.Rule{Target: verification.ErrInvalidReview, Code: codes.InvalidArgument},
		grpcx.Rule{Target: store.ErrNotFound, Code: codes.NotFound},
		grpcx.Rule{Target: store.ErrAlreadyReviewed, Code: codes.FailedPrecondition},
	)
	if err != nil {
		return nil, err
	}
	return &Handler{svc: svc, gate: auditx.NewGate(audit, logger), logger: logger, errs: errs}, nil
}

func Register(srv *grpc.Server, h *Handler) {
	verificationv1.RegisterVerificationServiceServer(srv, h)
}

func (h *Handler) fail(ctx context.Context, err error) error {
	return grpcx.Status(ctx, h.logger, err, h.errs)
}

func knownType(p auth.Principal) bool {
	return p.Type == auth.TypeUser || p.Type == auth.TypeService
}

func caller(ctx context.Context) (auth.Principal, error) {
	p, err := auth.RequirePrincipal(ctx)
	if err != nil {
		return auth.Principal{}, err
	}
	if !knownType(p) {
		return auth.Principal{}, status.Error(codes.Unauthenticated, "authentication required")
	}
	return p, nil
}

func (h *Handler) SubmitKyc(ctx context.Context, req *verificationv1.SubmitKycRequest) (*verificationv1.SubmitKycResponse, error) {
	if _, err := caller(ctx); err != nil {
		return nil, err
	}
	p, err := auth.RequireUser(ctx)
	if err != nil {
		return nil, err
	}
	sub, err := h.svc.Submit(ctx, p.ID, req.GetDocType(), req.GetDocRef())
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	return &verificationv1.SubmitKycResponse{Id: sub.ID, Status: toProtoStatus(sub.Status)}, nil
}

func (h *Handler) GetVerificationStatus(ctx context.Context, req *verificationv1.GetVerificationStatusRequest) (*verificationv1.GetVerificationStatusResponse, error) {
	p, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	userID := req.GetUserId()
	if userID == "" || userID == p.ID {
		if p, err = auth.RequireUser(ctx); err != nil {
			return nil, err
		}
		userID = p.ID
	} else if _, err = auth.RequireScopes(ctx, adminScope); err != nil {
		return nil, err
	}
	st, badge, err := h.svc.GetStatus(ctx, userID)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	return &verificationv1.GetVerificationStatusResponse{Status: toProtoStatus(st), Badge: badge}, nil
}

func (h *Handler) ReviewKyc(ctx context.Context, req *verificationv1.ReviewKycRequest) (*verificationv1.ReviewKycResponse, error) {
	if _, err := caller(ctx); err != nil {
		return nil, err
	}
	p, err := auth.RequireScopes(ctx, adminScope)
	if err != nil {
		return nil, err
	}
	sub, err := h.svc.Get(ctx, req.GetId())
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	if sub.UserID == p.ID {
		return nil, status.Error(codes.PermissionDenied, "reviewers cannot review their own submission")
	}
	decision, err := verification.NormalizeDecision(req.GetDecision())
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	if sub.Status != store.StatusPending {
		return nil, h.fail(ctx, store.ErrAlreadyReviewed)
	}
	ev := auditx.Event{
		ActorID:    p.ID,
		Action:     "kyc.review",
		TargetType: "kyc_submission",
		TargetID:   sub.ID,
		Metadata:   map[string]string{"user_id": sub.UserID, "decision": decision},
	}
	var st store.Status
	err = h.gate.Do(ctx, ev, func(ctx context.Context) error {
		var rerr error
		st, rerr = h.svc.Review(ctx, req.GetId(), decision)
		return rerr
	})
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	return &verificationv1.ReviewKycResponse{Status: toProtoStatus(st)}, nil
}

func toProtoStatus(s store.Status) verificationv1.VerificationStatus {
	switch s {
	case store.StatusVerified:
		return verificationv1.VerificationStatus_VERIFICATION_STATUS_VERIFIED
	case store.StatusRejected:
		return verificationv1.VerificationStatus_VERIFICATION_STATUS_REJECTED
	case store.StatusPending:
		return verificationv1.VerificationStatus_VERIFICATION_STATUS_PENDING
	default:
		return verificationv1.VerificationStatus_VERIFICATION_STATUS_PENDING
	}
}
