package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"
)

const (
	OutboxTable    = "payment_outbox_events"
	OutboxPurgeAge = 7 * 24 * time.Hour
)

type DBTX interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type SettleFn func(settled PaymentTransaction) (outbox.Row, error)

type PaymentTxWriter interface {
	SettleTx(ctx context.Context, id string, status PaymentStatus, providerRef string, ev SettleFn) (PaymentTransaction, error)
}

type PgTxWriter struct {
	pool   *pgxpool.Pool
	outbox *outbox.Store
}

func NewPgTxWriter(pool *pgxpool.Pool, ob *outbox.Store) *PgTxWriter {
	return &PgTxWriter{pool: pool, outbox: ob}
}

func (w *PgTxWriter) SettleTx(ctx context.Context, id string, status PaymentStatus, providerRef string, ev SettleFn) (PaymentTransaction, error) {
	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return PaymentTransaction{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	updated, err := updatePaymentStatusTx(ctx, tx, id, status, providerRef)
	if err != nil {
		return PaymentTransaction{}, err
	}
	row, err := ev(updated)
	if err != nil {
		return PaymentTransaction{}, err
	}
	if err := w.outbox.EnqueueTx(ctx, tx, row); err != nil {
		return PaymentTransaction{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return PaymentTransaction{}, fmt.Errorf("commit tx: %w", err)
	}
	return updated, nil
}

func PurgeOutbox(ctx context.Context, ob *outbox.Store) (int64, error) {
	return ob.Purge(ctx, OutboxPurgeAge)
}

var _ PaymentTxWriter = (*PgTxWriter)(nil)

// updatePaymentStatusTx runs the status UPDATE over any DBTX (pool or tx) and
// returns the updated row.
func updatePaymentStatusTx(ctx context.Context, q DBTX, id string, status PaymentStatus, providerRef string) (PaymentTransaction, error) {
	// compare-and-set: only a PENDING (1) or FAILED (3) payment is settled or failed,
	// so a racing call can never reopen a paid or refunded payment
	const sql = `UPDATE payment_transactions SET status = $1, provider_reference = $2, updated_at = NOW()
		WHERE id = $3 AND status IN (1, 3)
		RETURNING ` + txColumns
	var t PaymentTransaction
	if err := scanTransaction(q.QueryRow(ctx, sql, int32(status), providerRef, id), &t); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			var exists bool
			if qerr := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM payment_transactions WHERE id = $1)`, id).Scan(&exists); qerr != nil {
				return PaymentTransaction{}, fmt.Errorf("check payment tx: %w", qerr)
			}
			if exists {
				return PaymentTransaction{}, ErrNotSettleable
			}
			return PaymentTransaction{}, ErrTransactionNotFound
		}
		return PaymentTransaction{}, fmt.Errorf("update payment status: %w", err)
	}
	return t, nil
}
