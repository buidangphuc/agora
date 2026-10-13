package consume

import (
	"context"
	"fmt"
	"regexp"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const defaultDedupeTable = "processed_events"

var tableName = regexp.MustCompile(`^[a-z_][a-z0-9_]*$`)

// Querier runs one single-row query. *pgxpool.Pool, *pgxpool.Conn and pgx.Tx
// satisfy it.
type Querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Executor runs one statement. *pgxpool.Pool, *pgxpool.Conn and pgx.Tx satisfy
// it; pass the transaction that holds the handler's writes.
type Executor interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// Dedupe is a per-consumer processed-event ledger. Call Seen inside the same
// transaction as the handler's writes, so a rollback also un-marks the event.
// The zero value uses the default table "processed_events".
type Dedupe struct{ table string }

// NewDedupe returns a ledger over table; "" means "processed_events". It fails
// unless the name matches ^[a-z_][a-z0-9_]*$, so the name is safe to place in SQL.
func NewDedupe(table string) (Dedupe, error) {
	if table == "" {
		return Dedupe{}, nil
	}
	if !tableName.MatchString(table) {
		return Dedupe{}, fmt.Errorf("consume: invalid dedupe table %q", table)
	}
	return Dedupe{table: table}, nil
}

func (d Dedupe) name() string {
	if d.table == "" {
		return defaultDedupeTable
	}
	return d.table
}

// Seen records (consumer, eventID) with one INSERT ... ON CONFLICT DO NOTHING
// through ex. It returns true when the pair was already recorded (a duplicate to
// skip) and false when this call inserted it. The same eventID is independent
// across consumers.
func (d Dedupe) Seen(ctx context.Context, ex Executor, consumer, eventID string) (bool, error) {
	tag, err := ex.Exec(ctx,
		"INSERT INTO "+d.name()+" (consumer, event_id) VALUES ($1, $2) ON CONFLICT DO NOTHING",
		consumer, eventID)
	if err != nil {
		return false, fmt.Errorf("consume: mark %s/%s processed: %w", consumer, eventID, err)
	}
	return tag.RowsAffected() == 0, nil
}

// Processed reports whether (consumer, eventID) is already recorded, without
// inserting. q may be a pool or a transaction. Use it when the event is marked
// with Seen only after a side effect; a database error is returned, never read
// as "not processed".
func (d Dedupe) Processed(ctx context.Context, q Querier, consumer, eventID string) (bool, error) {
	var exists bool
	err := q.QueryRow(ctx,
		"SELECT EXISTS (SELECT 1 FROM "+d.name()+" WHERE consumer = $1 AND event_id = $2)",
		consumer, eventID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("consume: check %s/%s processed: %w", consumer, eventID, err)
	}
	return exists, nil
}

// DedupeSchemaSQL returns the CREATE TABLE for the ledger. table is validated as
// in NewDedupe; "" means "processed_events".
func DedupeSchemaSQL(table string) (string, error) {
	d, err := NewDedupe(table)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
    consumer     TEXT        NOT NULL,
    event_id     TEXT        NOT NULL,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (consumer, event_id)
)`, d.name()), nil
}
