package service

// TEST-STUB (task 6): compile-only declarations so listing_test.go builds
// before the implementation exists. The implementer replaces this file by
// moving these declarations into listing.go with real behaviour, then deletes it.

import (
	"context"
	"errors"

	aiv1 "github.com/buidangphuc/team-domain/generated/platform/ai/v1"
	"github.com/buidangphuc/team-domain/internal/upstream"
)

// ErrStatusLocked: the listing is BANNED or HELD; seller writes and deletes are refused.
var ErrStatusLocked = errors.New("listing status is locked")

// KeywordMatcher is the local prohibited-term screen (*screen.Screener satisfies it).
type KeywordMatcher interface {
	Match(text string) []string
}

// Screener is the AI screen (*upstream.AIClient satisfies it).
type Screener interface {
	ScreenListing(ctx context.Context, in upstream.ScreenInput) (*aiv1.ScreenListingResponse, error)
}

// WithScreener wires the keyword matcher and the AI screener (either may be nil).
func (s *ListingService) WithScreener(kw KeywordMatcher, ai Screener) *ListingService { return s }
