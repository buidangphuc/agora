package outbox

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel/propagation"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/reqctx"
)

// ErrNotFound is returned when a mark targets a row that does not exist.
var ErrNotFound = errors.New("outbox row not found")

var tableRe = regexp.MustCompile(`^[a-z_][a-z0-9_]*$`)

func mustTable(table string) string {
	if !tableRe.MatchString(table) {
		panic(fmt.Sprintf("outbox: invalid table name %q", table))
	}
	return table
}

// ErrEmptyEventID is returned by EnqueueTx for a Row without an EventID. The
// event id is the idempotency key, so an empty one would be silently dropped.
var ErrEmptyEventID = errors.New("outbox: empty event id")

// Row is one event to enqueue. Payload is published verbatim, keyed by AggregateID.
type Row struct {
	EventID       string
	AggregateType string
	AggregateID   string
	EventType     string
	Payload       []byte
	// RequestID is published as the x-request-id header. When empty, EnqueueTx
	// takes it from the context (reqctx.RequestID).
	RequestID string
	// Topic overrides the relayer's default topic for this row. Empty means the
	// relayer's Topic.
	Topic string
}

// pending is a claimed row.
type pending struct {
	eventID, aggregateID, eventType string
	topic, requestID, traceparent   string
	payload                         []byte
	attempts                        int
}

// Store reads and writes one outbox table.
type Store struct {
	pool  *pgxpool.Pool
	table string
}

// NewStore returns a store over an existing table with the SchemaSQL columns.
// It panics when table is not a plain lowercase identifier, because the name is
// placed into SQL.
func NewStore(pool *pgxpool.Pool, table string) *Store {
	return &Store{pool: pool, table: mustTable(table)}
}

// SchemaSQL returns the CREATE TABLE and index statements for an outbox table.
// It panics on an invalid table name.
//
// The statements are idempotent and additive: on a table created by an older
// SDK they only add the missing columns and indexes. A service that applies its
// own migrations instead must add to an existing table:
//
//	ALTER TABLE <table> ADD COLUMN IF NOT EXISTS topic       TEXT;
//	ALTER TABLE <table> ADD COLUMN IF NOT EXISTS traceparent TEXT;
//	ALTER TABLE <table> ADD COLUMN IF NOT EXISTS seq         BIGSERIAL;
//	CREATE INDEX IF NOT EXISTS <table>_agg_idx ON <table> (aggregate_type, aggregate_id, seq) WHERE status <> 'published';
//	CREATE INDEX IF NOT EXISTS <table>_pending_idx ON <table> (seq) WHERE status = 'pending';
//
// seq orders events: created_at is the transaction start, so events enqueued in
// one transaction tie on it. The old principal column is no longer written and
// may stay or be dropped. New tables do not have it.
func SchemaSQL(table string) string {
	t := mustTable(table)
	return fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %[1]s (
    event_id       TEXT PRIMARY KEY,
    aggregate_type TEXT        NOT NULL,
    aggregate_id   TEXT        NOT NULL,
    event_type     TEXT        NOT NULL,
    payload        BYTEA       NOT NULL,
    topic          TEXT,
    request_id     TEXT        NOT NULL DEFAULT '',
    traceparent    TEXT,
    seq            BIGSERIAL,
    status         TEXT        NOT NULL DEFAULT 'pending',
    attempts       INT         NOT NULL DEFAULT 0,
    available_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    locked_until   TIMESTAMPTZ,
    published_at   TIMESTAMPTZ,
    error          TEXT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
ALTER TABLE %[1]s ADD COLUMN IF NOT EXISTS topic       TEXT;
ALTER TABLE %[1]s ADD COLUMN IF NOT EXISTS traceparent TEXT;
ALTER TABLE %[1]s ADD COLUMN IF NOT EXISTS seq         BIGSERIAL;
CREATE INDEX IF NOT EXISTS %[1]s_claim_idx ON %[1]s (available_at, created_at) WHERE status = 'pending';
CREATE INDEX IF NOT EXISTS %[1]s_status_idx ON %[1]s (status);
CREATE INDEX IF NOT EXISTS %[1]s_agg_idx ON %[1]s (aggregate_type, aggregate_id, seq) WHERE status <> 'published';
CREATE INDEX IF NOT EXISTS %[1]s_pending_idx ON %[1]s (seq) WHERE status = 'pending';
`, t)
}

// EnqueueTx inserts a pending row inside tx, so it commits or rolls back with the
// caller's business write. Enqueuing an event id that already exists is a no-op,
// so a retried business write neither fails nor duplicates the event. An empty
// EventID is rejected with ErrEmptyEventID.
//
// The W3C traceparent of ctx is stored with the row and republished as a
// header, so the consumer's trace continues the producer's. An empty
// Row.RequestID defaults to the request id in ctx.
func (s *Store) EnqueueTx(ctx context.Context, tx pgx.Tx, r Row) error {
	if r.EventID == "" {
		return ErrEmptyEventID
	}
	if r.RequestID == "" {
		r.RequestID = reqctx.RequestID(ctx)
	}
	carrier := propagation.MapCarrier{}
	propagation.TraceContext{}.Inject(ctx, carrier)
	q := `INSERT INTO ` + s.table + `
		(event_id, aggregate_type, aggregate_id, event_type, payload, topic, request_id, traceparent, status, attempts, available_at, created_at)
		VALUES ($1, $2, $3, $4, $5, NULLIF($6, ''), $7, NULLIF($8, ''), 'pending', 0, now(), now())
		ON CONFLICT (event_id) DO NOTHING`
	if _, err := tx.Exec(ctx, q, r.EventID, r.AggregateType, r.AggregateID, r.EventType, r.Payload, r.Topic, r.RequestID, carrier.Get("traceparent")); err != nil {
		return fmt.Errorf("enqueue outbox event %q: %w", r.EventID, err)
	}
	return nil
}

// Purge deletes published rows older than olderThan and returns how many it
// removed. Pending and parked rows are never deleted. Run it periodically; the
// table otherwise grows without bound.
func (s *Store) Purge(ctx context.Context, olderThan time.Duration) (int64, error) {
	tag, err := s.pool.Exec(ctx, `DELETE FROM `+s.table+`
		WHERE status = 'published' AND published_at < now() - ($1 * interval '1 second')`, olderThan.Seconds())
	if err != nil {
		return 0, fmt.Errorf("purge outbox: %w", err)
	}
	return tag.RowsAffected(), nil
}

// claim leases up to batch pending, available, unlocked rows for lock.
// Order is per aggregate: a row is never claimed while an older row of the same
// aggregate is unpublished, whether that row is backing off, leased by another
// relayer or parked as failed. So a pass claims at most one row per aggregate.
// FOR UPDATE SKIP LOCKED keeps concurrent relayers off each other's rows. The
// outer ORDER BY is needed because UPDATE ... RETURNING does not keep the
// subquery order.
func (s *Store) claim(ctx context.Context, batch int, lock time.Duration) ([]pending, error) {
	q := `
		WITH claimed AS (
			UPDATE ` + s.table + `
			SET locked_until = now() + ($2::float8 * interval '1 second')
			WHERE event_id IN (
				SELECT c.event_id FROM ` + s.table + ` c
				WHERE c.status = 'pending'
				  AND c.available_at <= now()
				  AND (c.locked_until IS NULL OR c.locked_until <= now())
				  AND NOT EXISTS (
				      SELECT 1 FROM ` + s.table + ` o
				      WHERE o.aggregate_type = c.aggregate_type
				        AND o.aggregate_id = c.aggregate_id
				        AND o.seq < c.seq
				        AND o.status <> 'published')
				ORDER BY c.seq
				FOR UPDATE OF c SKIP LOCKED
				LIMIT $1
			)
			RETURNING event_id, aggregate_id, event_type, payload, COALESCE(topic, '') AS topic, request_id, COALESCE(traceparent, '') AS traceparent, attempts, seq
		)
		SELECT event_id, aggregate_id, event_type, payload, topic, request_id, traceparent, attempts FROM claimed
		ORDER BY seq`
	rows, err := s.pool.Query(ctx, q, batch, lock.Seconds())
	if err != nil {
		return nil, fmt.Errorf("claim outbox rows: %w", err)
	}
	defer rows.Close()
	var out []pending
	for rows.Next() {
		var p pending
		if err := rows.Scan(&p.eventID, &p.aggregateID, &p.eventType, &p.payload, &p.topic, &p.requestID, &p.traceparent, &p.attempts); err != nil {
			return nil, fmt.Errorf("scan outbox row: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// release ends the lease on rows that were claimed but not attempted, so the
// next pass can take them at once instead of after the lease expires.
func (s *Store) release(ctx context.Context, eventIDs []string) error {
	if len(eventIDs) == 0 {
		return nil
	}
	if _, err := s.pool.Exec(ctx, `UPDATE `+s.table+` SET locked_until = NULL WHERE event_id = ANY($1) AND status = 'pending'`, eventIDs); err != nil {
		return fmt.Errorf("release outbox rows: %w", err)
	}
	return nil
}

func (s *Store) markPublished(ctx context.Context, eventID string) error {
	q := `UPDATE ` + s.table + `
		SET status = 'published', published_at = now(), locked_until = NULL, error = NULL
		WHERE event_id = $1`
	tag, err := s.pool.Exec(ctx, q, eventID)
	if err != nil {
		return fmt.Errorf("mark outbox %q published: %w", eventID, err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// markFailed records a failed publish: attempts+1, the error and the lease
// released. With a non-nil availableAt the row stays pending behind that
// backoff gate; with nil it is parked as failed.
func (s *Store) markFailed(ctx context.Context, eventID, reason string, availableAt *time.Time) error {
	var q string
	args := []any{eventID, reason}
	if availableAt != nil {
		q = `UPDATE ` + s.table + `
			SET status = 'pending', attempts = attempts + 1, error = $2, available_at = $3, locked_until = NULL
			WHERE event_id = $1`
		args = append(args, *availableAt)
	} else {
		q = `UPDATE ` + s.table + `
			SET status = 'failed', attempts = attempts + 1, error = $2, locked_until = NULL
			WHERE event_id = $1`
	}
	tag, err := s.pool.Exec(ctx, q, args...)
	if err != nil {
		return fmt.Errorf("mark outbox %q failed: %w", eventID, err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
