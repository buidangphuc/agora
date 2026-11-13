package store

import "context"

// RED-PHASE STUB: replaced by the implementer.
func (w *PgTxWriter) EraseSellerListings(ctx context.Context, userID, eventID string, enqueue EnqueueFn) (int, error) {
	return 0, nil
}
