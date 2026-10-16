package repository

// TEST-STUB (task 1, red phase): compile-only signatures so seller_hold_test.go
// builds. The implementer DELETES this file and implements the methods in
// seller_hold.go (same signatures).

import (
	"context"
	"errors"
	"time"
)

var errSellerHoldNotImplemented = errors.New("seller hold: not implemented")

// HoldSellerListings holds a seller's published/pending_review listings for one
// UserStatusChanged event; it returns how many listings changed.
func (w *PgTxWriter) HoldSellerListings(ctx context.Context, userID, eventID string, changedAt time.Time, enqueue EnqueueFn) (int, error) {
	return 0, errSellerHoldNotImplemented
}

// RestoreSellerListings returns a seller's held listings to held_prior_status;
// it returns how many listings changed.
func (w *PgTxWriter) RestoreSellerListings(ctx context.Context, userID, eventID string, changedAt time.Time, enqueue EnqueueFn) (int, error) {
	return 0, errSellerHoldNotImplemented
}
