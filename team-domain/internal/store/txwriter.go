package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"
)

type OutboxRow = outbox.Row

type EnqueueFn func(written Listing) (OutboxRow, error)

type TxWriter interface {
	CreateTx(ctx context.Context, l Listing, ev EnqueueFn) (Listing, error)
	UpdateTx(ctx context.Context, l Listing, ev EnqueueFn) (Listing, error)
	DeleteTx(ctx context.Context, id string, ev EnqueueFn) (Listing, error)
}

type PgTxWriter struct {
	pool   *pgxpool.Pool
	outbox *outbox.Store
}

func NewPgTxWriter(pool *pgxpool.Pool, outbox *outbox.Store) *PgTxWriter {
	return &PgTxWriter{pool: pool, outbox: outbox}
}

func (w *PgTxWriter) CreateTx(ctx context.Context, l Listing, ev EnqueueFn) (Listing, error) {
	return w.run(ctx, ev, func(tx pgx.Tx) (Listing, error) { return createListing(ctx, tx, l) })
}

func (w *PgTxWriter) UpdateTx(ctx context.Context, l Listing, ev EnqueueFn) (Listing, error) {
	return w.run(ctx, ev, func(tx pgx.Tx) (Listing, error) { return updateListing(ctx, tx, l) })
}

func (w *PgTxWriter) ModerateTx(ctx context.Context, id, fromStatus string, loadedUpdatedAt time.Time, d ModerationUpdate, ev EnqueueFn) (Listing, error) {
	return w.run(ctx, ev, func(tx pgx.Tx) (Listing, error) {
		return moderateListing(ctx, tx, id, fromStatus, loadedUpdatedAt, d)
	})
}

func (w *PgTxWriter) DeleteTx(ctx context.Context, id string, ev EnqueueFn) (Listing, error) {
	return w.run(ctx, ev, func(tx pgx.Tx) (Listing, error) { return deleteListing(ctx, tx, id) })
}

func (w *PgTxWriter) run(ctx context.Context, ev EnqueueFn, write func(pgx.Tx) (Listing, error)) (Listing, error) {
	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return Listing{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	out, err := write(tx)
	if err != nil {
		return Listing{}, err
	}
	row, err := ev(out)
	if err != nil {
		return Listing{}, err
	}
	if err := w.outbox.EnqueueTx(ctx, tx, row); err != nil {
		return Listing{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Listing{}, fmt.Errorf("commit tx: %w", err)
	}
	return out, nil
}

var _ TxWriter = (*PgTxWriter)(nil)
