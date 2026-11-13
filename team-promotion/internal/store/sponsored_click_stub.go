package store

import (
	"context"
	"errors"
	"time"
)

// STUB (test author): compile-only placeholder for T2-click-charge-and-impression-purge.
// The implementer replaces this file: move the method into the AdCampaignRepository
// interface, the Postgres repository and storefake, and delete the stub.

var ErrImpressionNotFound = errors.New("impression token not found")

type ClickResult struct {
	ListingID string
	Charged   bool
}

// RecordClick consumes token at instant now and charges the campaign's bid in one
// transaction. principalID is the forwarded caller ("" when anonymous); a principal
// equal to the campaign's seller is never charged and leaves the token unconsumed.
func (r *PostgresAdCampaignRepository) RecordClick(_ context.Context, _ string, _ string, _ time.Time) (ClickResult, error) {
	return ClickResult{}, errors.New("RecordClick not implemented")
}
