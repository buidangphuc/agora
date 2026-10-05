package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DBTX is the slice of pgx.Tx / pgxpool.Pool the outbox enqueue needs.
type DBTX interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// ErrOutboxNotFound is returned when a MarkPublished/MarkFailed targets a row
// that no longer exists.
var ErrOutboxNotFound = errors.New("outbox event not found")

// OutboxStore is the Postgres transactional-outbox store. EnqueueTx runs inside
// a caller-owned transaction (so the event commits atomically with the revoke);
// the claim/mark methods run on the pool and drive the relayer. Same shape as
// team-domain's OutboxStore.
type OutboxStore struct {
	pool *pgxpool.Pool
}

// NewOutboxStore builds the store over the shared pool.
func NewOutboxStore(pool *pgxpool.Pool) *OutboxStore {
	return &OutboxStore{pool: pool}
}

// EnqueueTx inserts one pending outbox row using the supplied transaction, so it
// commits or rolls back together with the session revoke on that same tx.
func (s *OutboxStore) EnqueueTx(ctx context.Context, tx DBTX, row OutboxRow) error {
	return enqueueTx(ctx, tx, row)
}

func enqueueTx(ctx context.Context, tx DBTX, row OutboxRow) error {
	const q = `INSERT INTO identity_outbox_events
		(event_id, aggregate_type, aggregate_id, event_type, payload, request_id, status, attempts, available_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, 'pending', 0, now(), now())`
	if _, err := tx.Exec(ctx, q, row.EventID, row.AggregateType, row.AggregateID, row.EventType, row.Payload, row.RequestID); err != nil {
		return fmt.Errorf("enqueue outbox event %q: %w", row.EventID, err)
	}
	return nil
}

// ClaimPending atomically leases up to batch pending rows whose backoff gate has
// elapsed, using FOR UPDATE SKIP LOCKED so multiple relayers never claim the
// same row. The lease (locked_until) simply expires if the relayer dies mid-batch,
// making the row re-claimable. Rows are ordered (available_at, created_at) to
// keep same-listing events in insert order.
func (s *OutboxStore) ClaimPending(ctx context.Context, batch, lockSeconds int) ([]PendingEvent, error) {
	if batch <= 0 {
		return nil, errors.New("batch must be positive")
	}
	if lockSeconds <= 0 {
		return nil, errors.New("lockSeconds must be positive")
	}
	// UPDATE ... RETURNING does not preserve the subquery's ORDER BY, so the claimed
	// rows are re-sorted in the outer SELECT; otherwise two events for the same
	// aggregate in one batch could be produced newest-first.
	const q = `
		WITH claimed AS (
			UPDATE identity_outbox_events
			SET locked_until = now() + ($2 * interval '1 second')
			WHERE event_id IN (
				SELECT event_id FROM identity_outbox_events
				WHERE status = 'pending'
				  AND available_at <= now()
				  AND (locked_until IS NULL OR locked_until <= now())
				ORDER BY available_at, created_at
				FOR UPDATE SKIP LOCKED
				LIMIT $1
			)
			RETURNING event_id, aggregate_id, payload, attempts, available_at, created_at
		)
		SELECT event_id, aggregate_id, payload, attempts FROM claimed
		ORDER BY available_at, created_at, event_id`
	rows, err := s.pool.Query(ctx, q, batch, lockSeconds)
	if err != nil {
		return nil, fmt.Errorf("claim pending outbox rows: %w", err)
	}
	defer rows.Close()

	var out []PendingEvent
	for rows.Next() {
		var e PendingEvent
		if err := rows.Scan(&e.EventID, &e.AggregateID, &e.Payload, &e.Attempts); err != nil {
			return nil, fmt.Errorf("scan pending outbox row: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// MarkPublished records a successful produce: status=published, lease released.
func (s *OutboxStore) MarkPublished(ctx context.Context, eventID string, publishedAt time.Time) error {
	const q = `UPDATE identity_outbox_events
		SET status = 'published', published_at = $2, locked_until = NULL, error = NULL
		WHERE event_id = $1`
	tag, err := s.pool.Exec(ctx, q, eventID, publishedAt)
	if err != nil {
		return fmt.Errorf("mark outbox %q published: %w", eventID, err)
	}
	if tag.RowsAffected() == 0 {
		return ErrOutboxNotFound
	}
	return nil
}

// MarkFailed records a produce failure. When availableAt is non-nil the row is
// scheduled for retry (status stays 'pending', available_at is the backoff
// gate); when nil the row is parked in 'failed' (max attempts exhausted). Either
// way attempts is incremented and the lease released, so a poison row never
// blocks the queue.
func (s *OutboxStore) MarkFailed(ctx context.Context, eventID, reason string, availableAt *time.Time) error {
	var q string
	var args []any
	if availableAt != nil {
		q = `UPDATE identity_outbox_events
			SET status = 'pending', attempts = attempts + 1, error = $2, available_at = $3, locked_until = NULL
			WHERE event_id = $1`
		args = []any{eventID, reason, *availableAt}
	} else {
		q = `UPDATE identity_outbox_events
			SET status = 'failed', attempts = attempts + 1, error = $2, locked_until = NULL
			WHERE event_id = $1`
		args = []any{eventID, reason}
	}
	tag, err := s.pool.Exec(ctx, q, args...)
	if err != nil {
		return fmt.Errorf("mark outbox %q failed: %w", eventID, err)
	}
	if tag.RowsAffected() == 0 {
		return ErrOutboxNotFound
	}
	return nil
}
