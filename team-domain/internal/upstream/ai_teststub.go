package upstream

// TEST-STUB: compile-only placeholder for task 5. The implementer replaces this
// file's contents with the real ai.go (delete this file when ai.go lands).

import (
	"context"
	"errors"
	"time"

	aiv1 "github.com/buidangphuc/team-domain/generated/platform/ai/v1"
)

// AIClient calls team-ai's AIService.ScreenListing.
type AIClient struct{}

// ScreenInput is the listing text sent for screening.
type ScreenInput struct {
	ListingID, Title, Description, CategoryID string
}

// NewAIClient dials addr (UPSTREAM_AI_ADDR). Empty addr returns (nil, nil).
func NewAIClient(addr string, callTimeout time.Duration) (*AIClient, error) {
	return &AIClient{}, nil // wrong on purpose: stub
}

// Close releases the connection. Nil-safe.
func (c *AIClient) Close() error { return nil }

// ScreenListing returns team-ai's verdict; any failure is an error (caller fails safe).
func (c *AIClient) ScreenListing(ctx context.Context, in ScreenInput) (*aiv1.ScreenListingResponse, error) {
	return nil, errors.New("stub")
}
