package store

import (
	"context"
	"errors"
	"sync"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/outbox"
)

// OutboxRow is the enqueue input: one pending payment event persisted in the SAME
// transaction as the payment-status write. Payload is the fully-marshalled
// platform.events.v1.EventEnvelope (published verbatim by the SDK relayer), EventID is
// that envelope's event_id (the stable dedupe key consumers use), AggregateID is the
// order id and the Kafka partition key.
type OutboxRow = outbox.Row

// SettleFn builds the outbox row for a just-settled payment. The transactional
// writer calls it INSIDE the write transaction (with the updated transaction),
// so the handler can construct the EventEnvelope from the final row while the
// status write and the outbox INSERT still commit atomically. Returning an error
// rolls the whole transaction back.
type SettleFn func(settled PaymentTransaction) (OutboxRow, error)

// PaymentTxWriter settles a payment (status + provider ref) and enqueues its
// outbox event in ONE transaction. The Postgres implementation (PgTxWriter)
// opens a pgx.Tx; the in-memory fake (InMemoryTxWriter) is used by black-box
// tests.
type PaymentTxWriter interface {
	SettleTx(ctx context.Context, id string, status PaymentStatus, providerRef string, ev SettleFn) (PaymentTransaction, error)
}

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
	rows []OutboxRow
}

// NewInMemoryTxWriter wraps repo so reads observe the writes it commits.
func NewInMemoryTxWriter(repo *InMemoryPaymentRepository) *InMemoryTxWriter {
	return &InMemoryTxWriter{repo: repo}
}

// SettleTx updates the payment status and enqueues the built event atomically.
// FailWrite simulates a write failure before anything is recorded; an enqueue
// (ev) error restores the pre-settle transaction so the fake matches the
// Postgres tx rollback (neither the status change nor the row survives).
func (w *InMemoryTxWriter) SettleTx(ctx context.Context, id string, status PaymentStatus, providerRef string, ev SettleFn) (PaymentTransaction, error) {
	if w.FailWrite {
		return PaymentTransaction{}, errors.New("simulated write failure")
	}
	before, err := w.repo.GetTransaction(ctx, id)
	if err != nil {
		return PaymentTransaction{}, err
	}
	updated, err := w.repo.updateStatusFromSettleable(id, status, providerRef)
	if err != nil {
		return PaymentTransaction{}, err
	}
	row, err := ev(updated)
	if err != nil {
		// Roll back the status write so no partial effect remains.
		_, _ = w.repo.UpdateStatus(ctx, id, before.Status, before.ProviderReference)
		return PaymentTransaction{}, err
	}
	w.mu.Lock()
	w.rows = append(w.rows, row)
	w.mu.Unlock()
	return updated, nil
}

// Rows returns a copy of the enqueued outbox rows for assertions.
func (w *InMemoryTxWriter) Rows() []OutboxRow {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make([]OutboxRow, len(w.rows))
	copy(out, w.rows)
	return out
}

var _ PaymentTxWriter = (*InMemoryTxWriter)(nil)
