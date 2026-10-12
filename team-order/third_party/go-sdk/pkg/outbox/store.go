package outbox

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
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

// Row is one event to enqueue. Payload is published verbatim, keyed by AggregateID.
type Row struct {
	EventID       string
	AggregateType string
	AggregateID   string
	EventType     string
	Payload       []byte
	RequestID     string
}

// pending is a claimed row.
type pending struct {
	eventID, aggregateID string
	payload              []byte
	attempts             int
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
func SchemaSQL(table string) string {
	t := mustTable(table)
	return fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %[1]s (
    event_id       TEXT PRIMARY KEY,
    aggregate_type TEXT        NOT NULL,
    aggregate_id   TEXT        NOT NULL,
    event_type     TEXT        NOT NULL,
    payload        BYTEA       NOT NULL,
    principal      BYTEA,
    request_id     TEXT        NOT NULL DEFAULT '',
    status         TEXT        NOT NULL DEFAULT 'pending',
    attempts       INT         NOT NULL DEFAULT 0,
    available_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    locked_until   TIMESTAMPTZ,
    published_at   TIMESTAMPTZ,
    error          TEXT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS %[1]s_claim_idx ON %[1]s (available_at, created_at) WHERE status = 'pending';
CREATE INDEX IF NOT EXISTS %[1]s_status_idx ON %[1]s (status);
`, t)
}

// EnqueueTx inserts a pending row inside tx, so it commits or rolls back with the
// caller's business write. Enqueuing an event id that already exists is a no-op,
// so a retried business write neither fails nor duplicates the event.
func (s *Store) EnqueueTx(ctx context.Context, tx pgx.Tx, r Row) error {
	q := `INSERT INTO ` + s.table + `
		(event_id, aggregate_type, aggregate_id, event_type, payload, request_id, status, attempts, available_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, 'pending', 0, now(), now())
		ON CONFLICT (event_id) DO NOTHING`
	if _, err := tx.Exec(ctx, q, r.EventID, r.AggregateType, r.AggregateID, r.EventType, r.Payload, r.RequestID); err != nil {
		return fmt.Errorf("enqueue outbox event %q: %w", r.EventID, err)
	}
	return nil
}

// claim leases up to batch pending, available, unlocked rows for lockSeconds.
// FOR UPDATE SKIP LOCKED keeps concurrent relayers off each other's rows. The
// outer ORDER BY is needed because UPDATE ... RETURNING does not keep the
// subquery order, and same-aggregate events must go out oldest first.
func (s *Store) claim(ctx context.Context, batch, lockSeconds int) ([]pending, error) {
	q := `
		WITH claimed AS (
			UPDATE ` + s.table + `
			SET locked_until = now() + ($2 * interval '1 second')
			WHERE event_id IN (
				SELECT event_id FROM ` + s.table + `
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
		return nil, fmt.Errorf("claim outbox rows: %w", err)
	}
	defer rows.Close()
	var out []pending
	for rows.Next() {
		var p pending
		if err := rows.Scan(&p.eventID, &p.aggregateID, &p.payload, &p.attempts); err != nil {
			return nil, fmt.Errorf("scan outbox row: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
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
