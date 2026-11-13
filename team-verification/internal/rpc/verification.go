package rpc

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	commonv1 "github.com/buidangphuc/team-verification/generated/platform/common/v1"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auditx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/grpcx"
	verificationv1 "github.com/buidangphuc/team-verification/generated/platform/verification/v1"
	"github.com/buidangphuc/team-verification/internal/store"
	"github.com/buidangphuc/team-verification/internal/verification"
)

const (
	adminScope = "admin"
	readScope  = "verification.read"
)

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
		grpcx.Rule{Target: verification.ErrInvalidUpload, Code: codes.InvalidArgument},
		grpcx.Rule{Target: verification.ErrTooManyDocs, Code: codes.InvalidArgument},
		grpcx.Rule{Target: verification.ErrInvalidReason, Code: codes.InvalidArgument},
		grpcx.Rule{Target: verification.ErrInvalidStatus, Code: codes.InvalidArgument},
		grpcx.Rule{Target: verification.ErrInvalidCursor, Code: codes.InvalidArgument},
		grpcx.Rule{Target: verification.ErrInvalidIndex, Code: codes.InvalidArgument},
		grpcx.Rule{Target: verification.ErrInvalidIDs, Code: codes.InvalidArgument},
		grpcx.Rule{Target: store.ErrGrantInvalid, Code: codes.InvalidArgument},
		grpcx.Rule{Target: store.ErrGrantExpired, Code: codes.FailedPrecondition},
		grpcx.Rule{Target: store.ErrGrantCapReached, Code: codes.ResourceExhausted},
		grpcx.Rule{Target: verification.ErrObjectMissing, Code: codes.FailedPrecondition},
		grpcx.Rule{Target: verification.ErrObjectMismatch, Code: codes.FailedPrecondition},
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
	refs := append([]string{req.GetDocRef()}, req.GetAdditionalDocRefs()...)
	sub, err := h.svc.SubmitDocuments(ctx, p.ID, req.GetDocType(), refs)
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
	showReason := true
	if userID == "" || userID == p.ID {
		if p, err = auth.RequireUser(ctx); err != nil {
			return nil, err
		}
		userID = p.ID
	} else if p.Type == auth.TypeService {
		if _, err = auth.RequireScopes(ctx, readScope); err != nil {
			return nil, err
		}
		showReason = false
	} else if _, err = auth.RequireScopes(ctx, adminScope); err != nil {
		return nil, err
	}
	sub, err := h.svc.Latest(ctx, userID)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	if sub == nil {
		return &verificationv1.GetVerificationStatusResponse{Status: verificationv1.VerificationStatus_VERIFICATION_STATUS_NOT_SUBMITTED}, nil
	}
	resp := &verificationv1.GetVerificationStatusResponse{Status: toProtoStatus(sub.Status), Badge: sub.Status == store.StatusVerified}
	if showReason && sub.Status == store.StatusRejected {
		resp.RejectReasonCode = sub.ReasonCode
	}
	return resp, nil
}

func (h *Handler) CreateKycUploadUrl(ctx context.Context, req *verificationv1.CreateKycUploadUrlRequest) (*verificationv1.CreateKycUploadUrlResponse, error) {
	if _, err := caller(ctx); err != nil {
		return nil, err
	}
	p, err := auth.RequireUser(ctx)
	if err != nil {
		return nil, err
	}
	up, err := h.svc.CreateUpload(ctx, p.ID, req.GetContentType(), req.GetSizeBytes())
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	return &verificationv1.CreateKycUploadUrlResponse{UploadUrl: up.URL, DocRef: up.DocRef, ExpiresAt: timestamppb.New(up.ExpiresAt)}, nil
}

func (h *Handler) ListKycSubmissions(ctx context.Context, req *verificationv1.ListKycSubmissionsRequest) (*verificationv1.ListKycSubmissionsResponse, error) {
	if _, err := caller(ctx); err != nil {
		return nil, err
	}
	if _, err := auth.RequireScopes(ctx, adminScope); err != nil {
		return nil, err
	}
	var want store.Status
	switch req.GetStatus() {
	case verificationv1.VerificationStatus_VERIFICATION_STATUS_UNSPECIFIED:
	case verificationv1.VerificationStatus_VERIFICATION_STATUS_PENDING:
		want = store.StatusPending
	case verificationv1.VerificationStatus_VERIFICATION_STATUS_VERIFIED:
		want = store.StatusVerified
	case verificationv1.VerificationStatus_VERIFICATION_STATUS_REJECTED:
		want = store.StatusRejected
	default:
		return nil, h.fail(ctx, verification.ErrInvalidStatus)
	}
	page, err := h.svc.List(ctx, want, req.GetPage().GetCursor(), req.GetPage().GetPageSize())
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	out := make([]*verificationv1.KycSubmission, 0, len(page.Submissions))
	for _, s := range page.Submissions {
		m := &verificationv1.KycSubmission{
			Id: s.ID, UserId: s.UserID, DocType: s.DocType, Status: toProtoStatus(s.Status),
			DocumentCount: int32(s.DocumentCount), ReasonCode: s.ReasonCode, CreatedAt: timestamppb.New(s.CreatedAt),
		}
		if s.ReviewedAt != nil {
			m.ReviewedAt = timestamppb.New(*s.ReviewedAt)
		}
		out = append(out, m)
	}
	return &verificationv1.ListKycSubmissionsResponse{
		Submissions: out,
		Page:        &commonv1.PageResponse{NextCursor: page.NextCursor, Total: -1},
	}, nil
}

func (h *Handler) GetKycDocumentUrl(ctx context.Context, req *verificationv1.GetKycDocumentUrlRequest) (*verificationv1.GetKycDocumentUrlResponse, error) {
	if _, err := caller(ctx); err != nil {
		return nil, err
	}
	p, err := auth.RequireScopes(ctx, adminScope)
	if err != nil {
		return nil, err
	}
	sub, key, err := h.svc.DocumentKey(ctx, req.GetSubmissionId(), req.GetIndex())
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	ev := auditx.Event{
		ActorID:    p.ID,
		Action:     "kyc.document_view",
		TargetType: "kyc_submission",
		TargetID:   sub.ID,
		Metadata:   map[string]string{"user_id": sub.UserID, "index": strconv.Itoa(int(req.GetIndex()))},
	}
	var url string
	var exp time.Time
	err = h.gate.Do(ctx, ev, func(ctx context.Context) error {
		var uerr error
		url, exp, uerr = h.svc.DocumentURL(ctx, key)
		return uerr
	})
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	return &verificationv1.GetKycDocumentUrlResponse{Url: url, ExpiresAt: timestamppb.New(exp)}, nil
}

func (h *Handler) GetPublicBadges(ctx context.Context, req *verificationv1.GetPublicBadgesRequest) (*verificationv1.GetPublicBadgesResponse, error) {
	verified, ids, err := h.svc.Badges(ctx, req.GetUserIds())
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	out := make([]*verificationv1.PublicBadge, 0, len(ids))
	for _, id := range ids {
		out = append(out, &verificationv1.PublicBadge{UserId: id, Verified: verified[id]})
	}
	return &verificationv1.GetPublicBadgesResponse{Badges: out}, nil
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
	if err := verification.ValidateReason(decision, req.GetReasonCode()); err != nil {
		return nil, h.fail(ctx, err)
	}
	ev := auditx.Event{
		ActorID:    p.ID,
		Action:     "kyc.review",
		TargetType: "kyc_submission",
		TargetID:   sub.ID,
		Metadata:   map[string]string{"user_id": sub.UserID, "decision": decision, "reason_code": req.GetReasonCode()},
	}
	var st store.Status
	err = h.gate.Do(ctx, ev, func(ctx context.Context) error {
		var rerr error
		st, rerr = h.svc.ReviewWithReason(ctx, req.GetId(), decision, req.GetReasonCode())
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
