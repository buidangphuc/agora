package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/outbox"
)

// DBTX is the subset of pgxpool.Pool / pgx.Tx used by the transactional writer,
// so the same status-update SQL runs on either a pool or an open transaction.
type DBTX interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// PgTxWriter settles a payment and enqueues its outbox event in one pgx.Tx.
type PgTxWriter struct {
	pool   *pgxpool.Pool
	outbox *outbox.Store
}

// NewPgTxWriter builds the transactional writer over the pool and outbox store.
func NewPgTxWriter(pool *pgxpool.Pool, ob *outbox.Store) *PgTxWriter {
	return &PgTxWriter{pool: pool, outbox: ob}
}

// SettleTx opens a transaction, updates the payment status, enqueues the built
// event on the SAME tx, and commits — so the payment row and the outbox row are
// one atomic unit. Any error rolls both back (the deferred Rollback is a no-op
// after a successful Commit).
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

var _ PaymentTxWriter = (*PgTxWriter)(nil)
