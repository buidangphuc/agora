package repository

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ProcessedEventRepository is the consumer dedupe ledger (AD4/H3): it records the
// event_id of every event a consumer has already applied, so redelivery of the
// same event is a no-op. MarkProcessed reports whether the id was newly inserted
// (true = first time, caller should apply the effect) or already present (false =
// skip).
type ProcessedEventRepository interface {
	// MarkProcessed atomically records event_id for consumer. It returns
	// (true, nil) when the row was newly inserted (first delivery) and
	// (false, nil) when it already existed (duplicate — skip the effect).
	MarkProcessed(ctx context.Context, consumer, eventID string) (bool, error)
	// IsProcessed reports whether event_id was already applied by consumer.
	IsProcessed(ctx context.Context, consumer, eventID string) (bool, error)
}

// ── Postgres implementation ──

type PostgresProcessedEventRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresProcessedEventRepository(pool *pgxpool.Pool) *PostgresProcessedEventRepository {
	return &PostgresProcessedEventRepository{pool: pool}
}

func (r *PostgresProcessedEventRepository) MarkProcessed(ctx context.Context, consumer, eventID string) (bool, error) {
	// ON CONFLICT DO NOTHING makes the insert idempotent; RowsAffected() distinguishes
	// a fresh insert from a duplicate under concurrent delivery.
	const q = `INSERT INTO processed_events (event_id, consumer, processed_at)
		VALUES ($1, $2, $3) ON CONFLICT (event_id) DO NOTHING`
	ct, err := r.pool.Exec(ctx, q, eventID, consumer, time.Now())
	if err != nil {
		return false, fmt.Errorf("mark processed %q: %w", eventID, err)
	}
	return ct.RowsAffected() == 1, nil
}

func (r *PostgresProcessedEventRepository) IsProcessed(ctx context.Context, consumer, eventID string) (bool, error) {
	const q = `SELECT true FROM processed_events WHERE event_id = $1`
	var exists bool
	if err := r.pool.QueryRow(ctx, q, eventID).Scan(&exists); err != nil {
		// pgx returns ErrNoRows when absent; treat as not-processed.
		return false, nil
	}
	return exists, nil
}

// ── In-memory implementation ──

type InMemoryProcessedEventRepository struct {
	mu   sync.Mutex
	seen map[string]struct{}
}

func NewInMemoryProcessedEventRepository() *InMemoryProcessedEventRepository {
	return &InMemoryProcessedEventRepository{seen: make(map[string]struct{})}
}

func key(consumer, eventID string) string { return consumer + "\x00" + eventID }

func (r *InMemoryProcessedEventRepository) MarkProcessed(_ context.Context, consumer, eventID string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := key(consumer, eventID)
	if _, ok := r.seen[k]; ok {
		return false, nil
	}
	r.seen[k] = struct{}{}
	return true, nil
}

func (r *InMemoryProcessedEventRepository) IsProcessed(_ context.Context, consumer, eventID string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.seen[key(consumer, eventID)]
	return ok, nil
}
