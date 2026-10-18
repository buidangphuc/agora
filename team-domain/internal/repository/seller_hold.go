package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// sellerHoldBatch bounds how many listings one UPDATE touches.
const sellerHoldBatch = 500

// ReasonSellerEnforcement is the moderation reason on listings held because
// their seller was suspended or banned.
const ReasonSellerEnforcement = "seller_enforcement"

// HoldSellerListings holds a seller's published/pending_review listings for one
// UserStatusChanged event; it returns how many listings changed.
func (w *PgTxWriter) HoldSellerListings(ctx context.Context, userID, eventID string, changedAt time.Time, enqueue EnqueueFn) (int, error) {
	const sql = `UPDATE listings SET held_prior_status = status, status = 'held',
			moderation_reason_code = '` + ReasonSellerEnforcement + `', updated_at = now()
		WHERE id IN (SELECT id FROM listings
			WHERE seller_id = $1 AND status IN ('published', 'pending_review')
			ORDER BY id LIMIT $2 FOR UPDATE)
		RETURNING ` + listingColumns
	return w.applySellerStatus(ctx, userID, eventID, changedAt, enqueue, sql)
}

// RestoreSellerListings returns a seller's held listings to held_prior_status;
// it returns how many listings changed.
func (w *PgTxWriter) RestoreSellerListings(ctx context.Context, userID, eventID string, changedAt time.Time, enqueue EnqueueFn) (int, error) {
	const sql = `UPDATE listings SET status = held_prior_status, held_prior_status = NULL,
			moderation_reason_code = '', updated_at = now()
		WHERE id IN (SELECT id FROM listings
			WHERE seller_id = $1 AND status = 'held' AND held_prior_status IS NOT NULL
			ORDER BY id LIMIT $2 FOR UPDATE)
		RETURNING ` + listingColumns
	return w.applySellerStatus(ctx, userID, eventID, changedAt, enqueue, sql)
}

func (w *PgTxWriter) applySellerStatus(ctx context.Context, userID, eventID string, changedAt time.Time, enqueue EnqueueFn, batchSQL string) (int, error) {
	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(ctx, `INSERT INTO processed_events (event_id) VALUES ($1) ON CONFLICT DO NOTHING`, eventID)
	if err != nil {
		return 0, fmt.Errorf("record event: %w", err)
	}
	if tag.RowsAffected() == 0 { // redelivery
		return 0, tx.Commit(ctx)
	}
	tag, err = tx.Exec(ctx, `INSERT INTO seller_status_applied (user_id, changed_at) VALUES ($1, $2)
		ON CONFLICT (user_id) DO UPDATE SET changed_at = EXCLUDED.changed_at
		WHERE seller_status_applied.changed_at < EXCLUDED.changed_at`, userID, changedAt)
	if err != nil {
		return 0, fmt.Errorf("record seller status: %w", err)
	}
	if tag.RowsAffected() == 0 { // stale: older or equal changed_at
		return 0, tx.Commit(ctx)
	}

	total := 0
	for {
		batch, err := sellerBatch(ctx, tx, batchSQL, userID)
		if err != nil {
			return 0, err
		}
		for _, l := range batch {
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
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit tx: %w", err)
	}
	return total, nil
}

func sellerBatch(ctx context.Context, tx pgx.Tx, sql, userID string) ([]Listing, error) {
	rows, err := tx.Query(ctx, sql, userID, sellerHoldBatch)
	if err != nil {
		return nil, fmt.Errorf("seller status batch: %w", err)
	}
	var out []Listing
	for rows.Next() {
		var l Listing
		if err := scanListing(rows, &l); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, l)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		v, err := loadVariants(ctx, tx, out[i].ID)
		if err != nil {
			return nil, err
		}
		out[i].Variants = v
	}
	return out, nil
}

// PurgeProcessedEvents deletes dedupe rows processed before the cutoff and
// returns how many were removed.
func (r *PostgresListingRepository) PurgeProcessedEvents(ctx context.Context, before time.Time) (int, error) {
	tag, err := r.pool.Exec(ctx, `DELETE FROM processed_events WHERE processed_at < $1`, before)
	if err != nil {
		return 0, fmt.Errorf("purge processed_events: %w", err)
	}
	return int(tag.RowsAffected()), nil
}
