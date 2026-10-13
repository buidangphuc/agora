package store

import (
	"context"
	"errors"
	"sync"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/outbox"
)

// EnqueueFn builds the outbox row for a persisted listing. The transactional
// writer calls it INSIDE the write transaction (with the just-written listing),
// so the handler can construct the EventEnvelope from the final row while the
// business write and the outbox INSERT still commit atomically. Returning an
// error rolls the whole transaction back.
type EnqueueFn func(written Listing) (outbox.Row, error)

// TxWriter persists a listing write and its outbox event in one transaction.
// The Postgres implementation (PgTxWriter) opens a pgx.Tx; the in-memory fake
// (InMemoryTxWriter) is used by black-box tests.
type TxWriter interface {
	CreateTx(ctx context.Context, l Listing, ev EnqueueFn) (Listing, error)
	UpdateTx(ctx context.Context, l Listing, ev EnqueueFn) (Listing, error)
	DeleteTx(ctx context.Context, id string, ev EnqueueFn) (Listing, error)
}

// InMemoryTxWriter is a deterministic fake for tests. It writes through to an
// InMemoryListingRepository and records the enqueued rows, so a test can assert
// that a write and its event commit together — or, with FailWrite set, that a
// failed write leaves NO event behind (atomic rollback).
type InMemoryTxWriter struct {
	repo *InMemoryListingRepository

	// FailWrite makes the business write fail before anything is recorded,
	// exercising the rollback path (no listing, no outbox row).
	FailWrite bool

	mu   sync.Mutex
	rows []outbox.Row
}

// NewInMemoryTxWriter wraps repo so reads observe the writes it commits.
func NewInMemoryTxWriter(repo *InMemoryListingRepository) *InMemoryTxWriter {
	return &InMemoryTxWriter{repo: repo}
}

// CreateTx creates the listing and records its event, or neither.
func (w *InMemoryTxWriter) CreateTx(ctx context.Context, l Listing, ev EnqueueFn) (Listing, error) {
	if w.FailWrite {
		return Listing{}, errors.New("simulated write failure")
	}
	out, err := w.repo.Create(ctx, l)
	if err != nil {
		return Listing{}, err
	}
	return w.enqueue(out, ev)
}

// UpdateTx updates the listing and records its event, or neither.
func (w *InMemoryTxWriter) UpdateTx(ctx context.Context, l Listing, ev EnqueueFn) (Listing, error) {
	if w.FailWrite {
		return Listing{}, errors.New("simulated write failure")
	}
	out, err := w.repo.Update(ctx, l)
	if err != nil {
		return Listing{}, err
	}
	return w.enqueue(out, ev)
}

// DeleteTx deletes the listing and records its event, or neither.
func (w *InMemoryTxWriter) DeleteTx(ctx context.Context, id string, ev EnqueueFn) (Listing, error) {
	if w.FailWrite {
		return Listing{}, errors.New("simulated write failure")
	}
	out, err := w.repo.Delete(ctx, id)
	if err != nil {
		return Listing{}, err
	}
	return w.enqueue(out, ev)
}

// enqueue records the event only after the write succeeded; a build error is
// treated as a rollback (nothing recorded), mirroring the Postgres tx.
func (w *InMemoryTxWriter) enqueue(out Listing, ev EnqueueFn) (Listing, error) {
	row, err := ev(out)
	if err != nil {
		return Listing{}, err
	}
	w.mu.Lock()
	w.rows = append(w.rows, row)
	w.mu.Unlock()
	return out, nil
}

// Rows returns a copy of the enqueued outbox rows for assertions.
func (w *InMemoryTxWriter) Rows() []outbox.Row {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make([]outbox.Row, len(w.rows))
	copy(out, w.rows)
	return out
}

var _ TxWriter = (*InMemoryTxWriter)(nil)
