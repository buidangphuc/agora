package store

import (
	"context"
	"fmt"
)

// EraseSellerListings deletes every listing, the storefront, bundles and the
// status-applied row of an erased seller in one transaction, enqueuing one
// outbox row per deleted listing; it returns how many listings were deleted.
func (w *PgTxWriter) EraseSellerListings(ctx context.Context, userID, eventID string, enqueue EnqueueFn) (int, error) {
	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(ctx, `INSERT INTO processed_events (event_id) VALUES ($1) ON CONFLICT DO NOTHING`, eventID)
	if err != nil {
		return 0, fmt.Errorf("record event: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return 0, tx.Commit(ctx)
	}

	const batchSQL = `SELECT ` + listingColumns + ` FROM listings WHERE seller_id = $1 ORDER BY id LIMIT $2 FOR UPDATE`
	total := 0
	for {
		batch, err := sellerBatch(ctx, tx, batchSQL, userID)
		if err != nil {
			return 0, err
		}
		for _, l := range batch {
			if _, err := deleteListing(ctx, tx, l.ID); err != nil {
				return 0, err
			}
			row, err := enqueue(l)
			if err != nil {
				return 0, err
			}
			if err := w.outbox.EnqueueTx(ctx, tx, row); err != nil {
				return 0, err
			}
		}
		total += len(batch)
		if len(batch) < sellerHoldBatch {
			break
		}
	}
	for _, q := range []string{
		`DELETE FROM bundles WHERE seller_id = $1`,
		`DELETE FROM storefronts WHERE seller_id = $1`,
		`DELETE FROM seller_status_applied WHERE user_id = $1`,
	} {
		if _, err := tx.Exec(ctx, q, userID); err != nil {
			return 0, fmt.Errorf("delete seller shop data: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit tx: %w", err)
	}
	return total, nil
}
