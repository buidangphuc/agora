package ga4relay

import (
	"context"
	"database/sql"
	"sync"
	"time"
)

// ledgerTTL is how long a sent order stays in the ledger.
const ledgerTTL = 90 * 24 * time.Hour

// Ledger records orders already relayed to GA4.
type Ledger interface {
	Sent(ctx context.Context, orderID string) (bool, error)
	MarkSent(ctx context.Context, orderID string) error
}

type memoryLedger struct {
	mu  sync.Mutex
	now func() time.Time
	m   map[string]time.Time
}

// NewMemoryLedger returns an in-memory Ledger with 90d retention.
func NewMemoryLedger(now func() time.Time) Ledger {
	return &memoryLedger{now: now, m: map[string]time.Time{}}
}

func (l *memoryLedger) Sent(_ context.Context, orderID string) (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	exp, ok := l.m[orderID]
	if !ok {
		return false, nil
	}
	if !l.now().Before(exp) {
		delete(l.m, orderID)
		return false, nil
	}
	return true, nil
}

func (l *memoryLedger) MarkSent(_ context.Context, orderID string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.m[orderID] = l.now().Add(ledgerTTL)
	return nil
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
