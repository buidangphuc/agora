package repository

// TEST-STUB (task 3): compile-only stubs for the red tests. The implementer
// replaces ModerationUpdate/ModerateTx/ListModerationQueue with the real thing
// (and deletes this file).

import (
	"context"
	"errors"
	"time"
)

// ModerationUpdate is the admin decision persisted by ModerateTx.
type ModerationUpdate struct {
	ToStatus   string
	ReasonCode string
	Note       string
	DecidedAt  time.Time
}

func (w *PgTxWriter) ModerateTx(ctx context.Context, id, fromStatus string, loadedUpdatedAt time.Time, d ModerationUpdate, ev EnqueueFn) (Listing, error) {
	return Listing{}, errors.New("TEST-STUB: ModerateTx not implemented")
}

func (r *PostgresListingRepository) ListModerationQueue(ctx context.Context, cursor string, pageSize int32) (Page, error) {
	return Page{}, errors.New("TEST-STUB: ListModerationQueue not implemented")
}
