package store

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/postgres"
)

// OutboxTable is the transactional-outbox table (migration 0006, upgraded to the
// go-sdk v1 shape by migrations 0011 to 0013).
const OutboxTable = "outbox_events"

// PgTxWriter persists a listing write and its outbox event in one pgx.Tx.
type PgTxWriter struct {
	pool   *pgxpool.Pool
	outbox *outbox.Store
}

// NewPgTxWriter builds the transactional writer over the pool and outbox store.
func NewPgTxWriter(pool *pgxpool.Pool, ob *outbox.Store) *PgTxWriter {
	return &PgTxWriter{pool: pool, outbox: ob}
}

// CreateTx creates the listing and enqueues its event in one transaction.
func (w *PgTxWriter) CreateTx(ctx context.Context, l Listing, ev EnqueueFn) (Listing, error) {
	return w.run(ctx, ev, func(tx pgx.Tx) (Listing, error) { return createListing(ctx, tx, l) })
}

// UpdateTx updates the listing and enqueues its event in one transaction.
func (w *PgTxWriter) UpdateTx(ctx context.Context, l Listing, ev EnqueueFn) (Listing, error) {
	return w.run(ctx, ev, func(tx pgx.Tx) (Listing, error) { return updateListing(ctx, tx, l) })
}

// DeleteTx deletes the listing and enqueues its event in one transaction.
func (w *PgTxWriter) DeleteTx(ctx context.Context, id string, ev EnqueueFn) (Listing, error) {
	return w.run(ctx, ev, func(tx pgx.Tx) (Listing, error) { return deleteListing(ctx, tx, id) })
}

// run performs the business write, enqueues the built event on the SAME tx and
// commits, so the listing row and the outbox row are one atomic unit. Any error
// rolls both back.
func (w *PgTxWriter) run(ctx context.Context, ev EnqueueFn, write func(pgx.Tx) (Listing, error)) (Listing, error) {
	var out Listing
	err := postgres.WithTx(ctx, w.pool, func(tx pgx.Tx) error {
		var err error
		if out, err = write(tx); err != nil {
			return err
		}
		row, err := ev(out)
		if err != nil {
			return err
		}
		return w.outbox.EnqueueTx(ctx, tx, row)
	})
	if err != nil {
		return Listing{}, err
	}
	return out, nil
}

var _ TxWriter = (*PgTxWriter)(nil)
