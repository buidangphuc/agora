package consume

import (
	"context"
	"fmt"
	"regexp"

	"github.com/jackc/pgx/v5"
)

const defaultDedupeTable = "processed_events"

var tableName = regexp.MustCompile(`^[a-z_][a-z0-9_]*$`)

// Dedupe is a per-consumer processed-event ledger. Call Seen inside the same
// transaction as the handler's writes, so a rollback also un-marks the event.
type Dedupe struct{ table string }

// NewDedupe returns a ledger over table; "" means "processed_events". The name
// must match ^[a-z_][a-z0-9_]*$, otherwise Seen returns an error.
func NewDedupe(table string) Dedupe {
	if table == "" {
		table = defaultDedupeTable
	}
	return Dedupe{table: table}
}

// Seen records (consumer, eventID) with one INSERT ... ON CONFLICT DO NOTHING
// inside tx. It returns true when the pair was already recorded (a duplicate to
// skip) and false when this call inserted it. The same eventID is independent
// across consumers.
func (d Dedupe) Seen(ctx context.Context, tx pgx.Tx, consumer, eventID string) (bool, error) {
	if !tableName.MatchString(d.table) {
		return false, fmt.Errorf("consume: invalid dedupe table %q", d.table)
	}
	tag, err := tx.Exec(ctx,
		"INSERT INTO "+d.table+" (consumer, event_id) VALUES ($1, $2) ON CONFLICT DO NOTHING",
		consumer, eventID)
	if err != nil {
		return false, fmt.Errorf("consume: mark %s/%s processed: %w", consumer, eventID, err)
	}
	return tag.RowsAffected() == 0, nil
}

// DedupeSchemaSQL returns the CREATE TABLE for the ledger. table must be a name
// accepted by NewDedupe.
func DedupeSchemaSQL(table string) string {
	return fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
    consumer     TEXT        NOT NULL,
    event_id     TEXT        NOT NULL,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (consumer, event_id)
)`, table)
}
