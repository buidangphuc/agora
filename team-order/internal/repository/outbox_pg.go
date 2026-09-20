package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PgOutboxRepository is the PostgreSQL-backed outbox implementation using pgxpool.
type PgOutboxRepository struct {
	pool *pgxpool.Pool
}

// NewPgOutboxRepository constructs a Postgres outbox repository.
func NewPgOutboxRepository(pool *pgxpool.Pool) *PgOutboxRepository {
	return &PgOutboxRepository{pool: pool}
}

func (r *PgOutboxRepository) Enqueue(ctx context.Context, row OutboxRow) error {
	q := `
INSERT INTO order_outbox_events (
    event_id, aggregate_type, aggregate_id, event_type, payload, request_id, status, created_at
) VALUES ($1, $2, $3, $4, $5, $6, 'pending', now())
ON CONFLICT (event_id) DO NOTHING`

	_, err := r.pool.Exec(ctx, q,
		row.EventID,
		row.AggregateType,
		row.AggregateID,
		row.EventType,
		row.Payload,
		row.RequestID,
	)
	if err != nil {
		return fmt.Errorf("enqueue outbox event %q: %w", row.EventID, err)
	}
	return nil
}

// EnqueueTx enqueues an outbox row within an active pgx transaction.
func (r *PgOutboxRepository) EnqueueTx(ctx context.Context, tx pgx.Tx, row OutboxRow) error {
	q := `
INSERT INTO order_outbox_events (
    event_id, aggregate_type, aggregate_id, event_type, payload, request_id, status, created_at
) VALUES ($1, $2, $3, $4, $5, $6, 'pending', now())
ON CONFLICT (event_id) DO NOTHING`

	_, err := tx.Exec(ctx, q,
		row.EventID,
		row.AggregateType,
		row.AggregateID,
		row.EventType,
		row.Payload,
		row.RequestID,
	)
	if err != nil {
		return fmt.Errorf("enqueue outbox event tx %q: %w", row.EventID, err)
	}
	return nil
}

func (r *PgOutboxRepository) ClaimPending(ctx context.Context, limit int, leaseDuration time.Duration) ([]PendingEvent, error) {
	q := `
UPDATE order_outbox_events
SET locked_until = now() + $1::interval
WHERE event_id IN (
    SELECT event_id
    FROM order_outbox_events
    WHERE status = 'pending'
      AND available_at <= now()
      AND (locked_until IS NULL OR locked_until < now())
    ORDER BY created_at ASC
    LIMIT $2
    FOR UPDATE SKIP LOCKED
)
RETURNING event_id, aggregate_id, payload, attempts`

	rows, err := r.pool.Query(ctx, q, fmt.Sprintf("%d milliseconds", leaseDuration.Milliseconds()), limit)
	if err != nil {
		return nil, fmt.Errorf("claim pending outbox events: %w", err)
	}
	defer rows.Close()

	var events []PendingEvent
	for rows.Next() {
		var ev PendingEvent
		if err := rows.Scan(&ev.EventID, &ev.AggregateID, &ev.Payload, &ev.Attempts); err != nil {
			return nil, fmt.Errorf("scan claimed outbox event: %w", err)
		}
		events = append(events, ev)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate claimed outbox events: %w", err)
	}
	return events, nil
}

func (r *PgOutboxRepository) MarkPublished(ctx context.Context, eventID string) error {
	q := `
UPDATE order_outbox_events
SET status = 'published', published_at = now(), locked_until = NULL
WHERE event_id = $1`

	_, err := r.pool.Exec(ctx, q, eventID)
	if err != nil {
		return fmt.Errorf("mark published %q: %w", eventID, err)
	}
	return nil
}

func (r *PgOutboxRepository) MarkFailed(ctx context.Context, eventID string, errStr string, retryDelay time.Duration) error {
	q := `
UPDATE order_outbox_events
SET attempts = attempts + 1,
    available_at = now() + $2::interval,
    locked_until = NULL,
    error = $3
WHERE event_id = $1`

	_, err := r.pool.Exec(ctx, q, eventID, fmt.Sprintf("%d milliseconds", retryDelay.Milliseconds()), errStr)
	if err != nil {
		return fmt.Errorf("mark failed %q: %w", eventID, err)
	}
	return nil
}
