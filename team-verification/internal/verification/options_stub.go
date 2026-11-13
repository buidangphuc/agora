package verification

import (
	"time"

	"github.com/buidangphuc/team-verification/internal/objects"
)

// STUB for the red-first test commit: the implementer replaces these no-op
// options with real wiring (object store and clock used by the KYC RPCs).
type Option func(*VerificationService)

func WithObjects(_ objects.Store) Option { return func(*VerificationService) {} }

func WithClock(_ func() time.Time) Option { return func(*VerificationService) {} }
