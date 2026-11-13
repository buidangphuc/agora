package rpc

import (
	"context"
	"log/slog"

	identityv1 "github.com/buidangphuc/team-identity/generated/platform/identity/v1"
	"github.com/buidangphuc/team-identity/internal/config"
	"github.com/buidangphuc/team-identity/internal/store"
)

// RED-STUB: compile-only placeholder for task t2-register-privacy-authz; the implementer replaces this file.

type ConsentRepository interface {
	AppendConsents(ctx context.Context, userID string, in []store.ConsentInput) error
	ListConsents(ctx context.Context, userID, cursor string, limit int) ([]store.Consent, string, error)
}

type PrivacyHandler struct {
	identityv1.UnimplementedPrivacyServiceServer
}

func NewPrivacyHandler(_ ConsentRepository, _ config.LegalTerms, _ config.PrivacyPolicy, _ config.SellerTerms, _ *slog.Logger) *PrivacyHandler {
	return &PrivacyHandler{}
}
