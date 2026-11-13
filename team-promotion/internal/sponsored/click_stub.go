package sponsored

import (
	"context"
	"errors"
	"time"
)

// STUB (test author): compile-only placeholder for T2-click-charge-and-impression-purge.
// The implementer replaces this file with the real implementation.

type RecordClickParams struct {
	Token       string
	PrincipalID string
}

type ClickResult struct {
	ListingID string
	Charged   bool
}

func (s *Service) WithClock(_ func() time.Time) *Service { return s }

func (s *Service) RecordAdClick(_ context.Context, _ RecordClickParams) (ClickResult, error) {
	return ClickResult{}, errors.New("RecordAdClick not implemented")
}
