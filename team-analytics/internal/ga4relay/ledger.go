package ga4relay

import (
	"context"
	"database/sql"
	"time"
)

// ledgerTTL is how long a sent order stays in the ledger.
const ledgerTTL = 90 * 24 * time.Hour

// Ledger records orders already relayed to GA4.
type Ledger interface {
	Sent(ctx context.Context, orderID string) (bool, error)
	MarkSent(ctx context.Context, orderID string) error
}

type sqlLedger struct {
	db  *sql.DB
	now func() time.Time
}

// NewSQLLedger returns a durable Ledger backed by table ga4_relay_sent.
func NewSQLLedger(ctx context.Context, db *sql.DB, now func() time.Time) (Ledger, error) {
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS ga4_relay_sent (
		order_id VARCHAR PRIMARY KEY,
		expires_micros BIGINT NOT NULL)`); err != nil {
		return nil, err
	}
	return &sqlLedger{db: db, now: now}, nil
}

func (l *sqlLedger) Sent(ctx context.Context, orderID string) (bool, error) {
	var n int
	err := l.db.QueryRowContext(ctx,
		`SELECT count(*) FROM ga4_relay_sent WHERE order_id = ? AND expires_micros > ?`,
		orderID, l.now().UnixMicro()).Scan(&n)
	return n > 0, err
}

func (l *sqlLedger) MarkSent(ctx context.Context, orderID string) error {
	_, err := l.db.ExecContext(ctx,
		`INSERT INTO ga4_relay_sent (order_id, expires_micros) VALUES (?, ?)
		 ON CONFLICT (order_id) DO UPDATE SET expires_micros = excluded.expires_micros`,
		orderID, l.now().Add(ledgerTTL).UnixMicro())
	return err
}

// Purger physically deletes expired rows and returns how many were removed.
type Purger interface {
	Purge(ctx context.Context) (int64, error)
}

func (l *sqlLedger) Purge(ctx context.Context) (int64, error) {
	res, err := l.db.ExecContext(ctx, `DELETE FROM ga4_relay_sent WHERE expires_micros <= ?`, l.now().UnixMicro())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
