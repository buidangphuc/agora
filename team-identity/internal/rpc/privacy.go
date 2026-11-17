package rpc

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	commonv1 "github.com/buidangphuc/team-identity/generated/platform/common/v1"
	identityv1 "github.com/buidangphuc/team-identity/generated/platform/identity/v1"
	"github.com/buidangphuc/team-identity/internal/config"
	"github.com/buidangphuc/team-identity/internal/identity"
	"github.com/buidangphuc/team-identity/internal/store"
)

const (
	consentDefaultPage = 20
	consentMaxPage     = 100
	consentMaxRecords  = 6
	consentMaxVersion  = 32
)

var consentSources = map[string]bool{"banner": true, "register": true, "account": true, "seller_onboarding": true}

type ConsentRepository interface {
	AppendConsents(ctx context.Context, userID string, in []store.ConsentInput) error
	ListConsents(ctx context.Context, userID, cursor string, limit int) ([]store.Consent, string, error)
}

type PrivacyHandler struct {
	identityv1.UnimplementedPrivacyServiceServer
	repo     ConsentRepository
	terms    []string
	privacy  []string
	seller   []string
	logger   *slog.Logger
	deletion *identity.AccountDeletionService
	export   ExportRepository
}

func NewPrivacyHandler(repo ConsentRepository, terms config.LegalTerms, privacy config.PrivacyPolicy, seller config.SellerTerms, logger *slog.Logger) *PrivacyHandler {
	return &PrivacyHandler{repo: repo, terms: terms.Versions, privacy: privacy.Versions, seller: seller.Versions, logger: orDefault(logger)}
}

func invalidConsent() error { return status.Error(codes.InvalidArgument, "invalid consent records") }

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func (h *PrivacyHandler) RecordConsent(ctx context.Context, req *identityv1.RecordConsentRequest) (*identityv1.RecordConsentResponse, error) {
	p, err := auth.RequireUser(ctx)
	if err != nil {
		return nil, err
	}
	recs := req.GetRecords()
	if len(recs) == 0 || len(recs) > consentMaxRecords {
		return nil, invalidConsent()
	}
	seen := map[identityv1.ConsentPurpose]bool{}
	in := make([]store.ConsentInput, 0, len(recs))
	for _, r := range recs {
		if r == nil {
			return nil, invalidConsent()
		}
		purpose := r.GetPurpose()
		name, known := identityv1.ConsentPurpose_name[int32(purpose)]
		if !known || purpose == identityv1.ConsentPurpose_CONSENT_PURPOSE_UNSPECIFIED || seen[purpose] {
			return nil, invalidConsent()
		}
		seen[purpose] = true
		v := r.GetVersion()
		if v == "" || len(v) > consentMaxVersion || !consentSources[r.GetSource()] {
			return nil, invalidConsent()
		}
		switch purpose {
		case identityv1.ConsentPurpose_CONSENT_PURPOSE_TERMS:
			if !r.GetGranted() || !firstIs(h.terms, v) {
				return nil, invalidConsent()
			}
		case identityv1.ConsentPurpose_CONSENT_PURPOSE_PRIVACY_POLICY:
			if !r.GetGranted() || !firstIs(h.privacy, v) {
				return nil, invalidConsent()
			}
		case identityv1.ConsentPurpose_CONSENT_PURPOSE_SELLER_TERMS:
			if !r.GetGranted() || !contains(h.seller, v) {
				return nil, invalidConsent()
			}
		}
		in = append(in, store.ConsentInput{Purpose: strings.TrimPrefix(name, "CONSENT_PURPOSE_"), Granted: r.GetGranted(), Version: v, Source: r.GetSource()})
	}
	if err := h.repo.AppendConsents(ctx, p.ID, in); err != nil {
		h.logger.ErrorContext(ctx, "record consent failed", "op", "record consent", "error", err)
		return nil, status.Error(codes.Internal, "internal error")
	}
	return &identityv1.RecordConsentResponse{}, nil
}

func firstIs(list []string, v string) bool { return len(list) > 0 && list[0] == v }

func (h *PrivacyHandler) ListMyConsents(ctx context.Context, req *identityv1.ListMyConsentsRequest) (*identityv1.ListMyConsentsResponse, error) {
	p, err := auth.RequireUser(ctx)
	if err != nil {
		return nil, err
	}
	limit := int(req.GetPage().GetPageSize())
	if limit <= 0 {
		limit = consentDefaultPage
	}
	if limit > consentMaxPage {
		limit = consentMaxPage
	}
	rows, next, err := h.repo.ListConsents(ctx, p.ID, req.GetPage().GetCursor(), limit)
	if err != nil {
		if strings.HasPrefix(err.Error(), "invalid cursor") {
			return nil, status.Error(codes.InvalidArgument, "invalid cursor")
		}
		h.logger.ErrorContext(ctx, "list consents failed", "op", "list consents", "error", err)
		return nil, status.Error(codes.Internal, "internal error")
	}
	out := make([]*identityv1.ConsentRecord, 0, len(rows))
	for _, c := range rows {
		out = append(out, &identityv1.ConsentRecord{
			Purpose:    identityv1.ConsentPurpose(identityv1.ConsentPurpose_value["CONSENT_PURPOSE_"+c.Purpose]),
			Granted:    c.Granted,
			Version:    c.Version,
			Source:     c.Source,
			RecordedAt: timestamppb.New(c.RecordedAt),
		})
	}
	return &identityv1.ListMyConsentsResponse{Records: out, Page: &commonv1.PageResponse{NextCursor: next}}, nil
}

func (h *PrivacyHandler) WithAccountDeletion(svc *identity.AccountDeletionService) *PrivacyHandler {
	h.deletion = svc
	return h
}

func (h *PrivacyHandler) deletionUser(ctx context.Context) (auth.Principal, error) {
	p, err := auth.RequireUser(ctx)
	if err != nil {
		return auth.Principal{}, err
	}
	if h.deletion == nil {
		return auth.Principal{}, status.Error(codes.Unimplemented, "account deletion not enabled")
	}
	return p, nil
}

func (h *PrivacyHandler) RequestAccountDeletion(ctx context.Context, req *identityv1.RequestAccountDeletionRequest) (*identityv1.RequestAccountDeletionResponse, error) {
	p, err := h.deletionUser(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetPassword() == "" {
		return nil, status.Error(codes.InvalidArgument, "password is required")
	}
	var erase time.Time
	run := func(ctx context.Context) error {
		var rerr error
		erase, rerr = h.deletion.Request(ctx, p.ID, p.SessionID, req.GetPassword())
		return rerr
	}
	if p.HasScope(scopeAdmin) {
		err = h.deletion.Audited(ctx, p.ID, "account.deletion_request", run)
	} else {
		err = run(ctx)
	}
	if err != nil {
		if status.Code(err) == codes.Unavailable {
			return nil, err
		}
		var blocked *identity.DeletionBlockedError
		switch {
		case errors.Is(err, identity.ErrDeletionPasswordMismatch):
			return nil, status.Error(codes.PermissionDenied, "password does not match")
		case errors.As(err, &blocked):
			return nil, status.Error(codes.FailedPrecondition, "account cannot be deleted while there are: "+strings.Join(blocked.Categories, ", "))
		case errors.Is(err, identity.ErrObligationsUnavailable):
			h.logger.WarnContext(ctx, "obligation check failed", "op", "request account deletion", "error", err)
			return nil, status.Error(codes.Unavailable, "service unavailable")
		}
		return nil, statusErr(ctx, h.logger, err)
	}
	return &identityv1.RequestAccountDeletionResponse{EraseAfter: timestamppb.New(erase)}, nil
}

func (h *PrivacyHandler) CancelAccountDeletion(ctx context.Context, _ *identityv1.CancelAccountDeletionRequest) (*identityv1.CancelAccountDeletionResponse, error) {
	p, err := h.deletionUser(ctx)
	if err != nil {
		return nil, err
	}
	run := func(ctx context.Context) error { return h.deletion.Cancel(ctx, p.ID) }
	if p.HasScope(scopeAdmin) {
		err = h.deletion.Audited(ctx, p.ID, "account.deletion_cancel", run)
	} else {
		err = run(ctx)
	}
	if err != nil {
		if status.Code(err) == codes.Unavailable {
			return nil, err
		}
		if errors.Is(err, identity.ErrNoPendingDeletion) {
			return nil, status.Error(codes.FailedPrecondition, "no pending account deletion")
		}
		return nil, statusErr(ctx, h.logger, err)
	}
	return &identityv1.CancelAccountDeletionResponse{}, nil
}

func (h *PrivacyHandler) GetAccountDeletionStatus(ctx context.Context, _ *identityv1.GetAccountDeletionStatusRequest) (*identityv1.GetAccountDeletionStatusResponse, error) {
	p, err := h.deletionUser(ctx)
	if err != nil {
		return nil, err
	}
	erase, err := h.deletion.Status(ctx, p.ID)
	if err != nil {
		return nil, statusErr(ctx, h.logger, err)
	}
	if erase.IsZero() {
		return &identityv1.GetAccountDeletionStatusResponse{}, nil
	}
	return &identityv1.GetAccountDeletionStatusResponse{Pending: true, EraseAfter: timestamppb.New(erase)}, nil
}
