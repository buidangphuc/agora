package identity

import (
	"context"
	"errors"
	"time"

	identityv1 "github.com/buidangphuc/team-identity/generated/platform/identity/v1"
	"github.com/buidangphuc/team-identity/internal/store"
)

// TEST-STUB: compile-only placeholder for task 1.2.1; the implementer replaces
// this file with the real handoff.go.

type HandoffRepository interface {
	InsertHandoffCode(ctx context.Context, c store.HandoffCode, maxLive int, now time.Time, inTx func(context.Context) error) error
}

type MintResult struct {
	Code      string
	ExpiresAt time.Time
}

func (s *AuthService) WithHandoff(HandoffRepository) *AuthService { return s }

func (s *AuthService) WithHandoffTTL(time.Duration) *AuthService { return s }

func (s *AuthService) Mint(ctx context.Context, sourceSID string, target identityv1.HandoffApp) (MintResult, error) {
	return MintResult{}, errors.New("not implemented")
}
