package fake

import (
	"context"
	"errors"
	"sync"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/team-payment/internal/store"
)

// InMemoryTxWriter is a deterministic fake for tests. It writes through to an
// InMemoryPaymentRepository and records the enqueued rows, so a test can assert
// that a status write and its event commit together — or, with FailWrite set,
// that a failed write leaves NO event behind (atomic rollback).
type InMemoryTxWriter struct {
	repo *InMemoryPaymentRepository

	// FailWrite makes the business write fail before anything is recorded,
	// exercising the rollback path (no status change, no outbox row).
	FailWrite bool

	mu   sync.Mutex
	rows []outbox.Row
}

// NewInMemoryTxWriter wraps repo so reads observe the writes it commits.
func NewInMemoryTxWriter(repo *InMemoryPaymentRepository) *InMemoryTxWriter {
	return &InMemoryTxWriter{repo: repo}
}

// SettleTx updates the payment status and enqueues the built event atomically.
// FailWrite simulates a write failure before anything is recorded; an enqueue
// (ev) error restores the pre-settle transaction so the fake matches the
// Postgres tx rollback (neither the status change nor the row survives).
func (w *InMemoryTxWriter) SettleTx(ctx context.Context, id string, status store.PaymentStatus, providerRef string, ev store.SettleFn) (store.PaymentTransaction, error) {
	if w.FailWrite {
		return store.PaymentTransaction{}, errors.New("simulated write failure")
	}
	before, err := w.repo.GetTransaction(ctx, id)
	if err != nil {
		return store.PaymentTransaction{}, err
	}
	updated, err := w.repo.updateStatusFromSettleable(id, status, providerRef)
	if err != nil {
		return store.PaymentTransaction{}, err
	}
	row, err := ev(updated)
	if err != nil {
		// Roll back the status write so no partial effect remains.
		_, _ = w.repo.UpdateStatus(ctx, id, before.Status, before.ProviderReference)
		return store.PaymentTransaction{}, err
	}
	w.mu.Lock()
	w.rows = append(w.rows, row)
	w.mu.Unlock()
	return updated, nil
}

// Rows returns a copy of the enqueued outbox rows for assertions.
func (w *InMemoryTxWriter) Rows() []outbox.Row {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make([]outbox.Row, len(w.rows))
	copy(out, w.rows)
	return out
}

var _ store.PaymentTxWriter = (*InMemoryTxWriter)(nil)
