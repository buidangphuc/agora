package duckdb

import (
	"context"
	"errors"

	"github.com/buidangphuc/team-analytics/internal/warehouse"
)

// RED-STUB (analytics-user-erasure/T1): compile-only; the implementer replaces it.

// Option configures Open.
type Option func(*Writer)

// WithTombstoneSecret sets ERASURE_TOMBSTONE_SECRET for the HMAC of the tombstone.
func WithTombstoneSecret(secret string) Option { return func(*Writer) {} }

// EraseUser is not implemented yet.
func (w *Writer) EraseUser(context.Context, string) (warehouse.EraseCounts, error) {
	return nil, errors.New("not implemented")
}

var _ warehouse.UserEraser = (*Writer)(nil)
