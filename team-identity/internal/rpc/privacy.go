package rpc

import (
	"context"
	"log/slog"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	commonv1 "github.com/buidangphuc/team-identity/generated/platform/common/v1"
	identityv1 "github.com/buidangphuc/team-identity/generated/platform/identity/v1"
	"github.com/buidangphuc/team-identity/internal/config"
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
	repo    ConsentRepository
	terms   []string
	privacy []string
	seller  []string
	logger  *slog.Logger
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
