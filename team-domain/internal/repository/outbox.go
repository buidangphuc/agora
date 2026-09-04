package repository

import (
	"context"
	"errors"
	"sync"
	"time"
)

// OutboxRow is the enqueue input: one pending domain event to persist in the
// SAME transaction as the business write. Payload is the fully-marshalled
// platform.events.v1.EventEnvelope (produced verbatim by the relayer), and
// EventID is that envelope's event_id — the stable dedupe key consumers use.
type OutboxRow struct {
	EventID       string
	AggregateType string // 'Listing'
	AggregateID   string // listing_id; the Kafka partition key
	EventType     string // e.g. 'platform.listing.v1.ListingChanged'
	Payload       []byte // marshalled EventEnvelope bytes
	RequestID     string
}

// PendingEvent is a claimed outbox row the relayer will produce. Attempts is the
// number of prior failed relays, used to compute the retry backoff.
type PendingEvent struct {
	EventID     string
	AggregateID string
	Payload     []byte
	Attempts    int
}

// EnqueueFn builds the outbox row for a persisted listing. The transactional
// writer calls it INSIDE the write transaction (with the just-written listing),
// so the handler can construct the EventEnvelope from the final row while the
// business write and the outbox INSERT still commit atomically. Returning an
// error rolls the whole transaction back.
type EnqueueFn func(written Listing) (OutboxRow, error)

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
	rows []OutboxRow
}

// NewInMemoryTxWriter wraps repo so reads observe the writes it commits.
func NewInMemoryTxWriter(repo *InMemoryListingRepository) *InMemoryTxWriter {
	return &InMemoryTxWriter{repo: repo}
}

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
func (w *InMemoryTxWriter) Rows() []OutboxRow {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make([]OutboxRow, len(w.rows))
	copy(out, w.rows)
	return out
}

// InMemoryOutbox is a fake OutboxStore claim/mark surface for relayer tests.
// It is intentionally simple (no locking semantics) — the FOR UPDATE SKIP
// LOCKED behaviour is only meaningful against real Postgres.
type InMemoryOutbox struct {
	mu        sync.Mutex
	pending   []PendingEvent
	Published map[string]time.Time
	Failed    map[string]string   // event_id -> last error (parked)
	Retried   map[string]time.Time // event_id -> next available_at
	Attempts  map[string]int
}

func NewInMemoryOutbox(seed ...PendingEvent) *InMemoryOutbox {
	return &InMemoryOutbox{
		pending:   append([]PendingEvent(nil), seed...),
		Published: map[string]time.Time{},
		Failed:    map[string]string{},
		Retried:   map[string]time.Time{},
		Attempts:  map[string]int{},
	}
}

// ClaimPending returns (and removes) up to batch pending events. lockSeconds is
// accepted for interface parity but unused by the fake.
func (o *InMemoryOutbox) ClaimPending(_ context.Context, batch, _ int) ([]PendingEvent, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if batch > len(o.pending) {
		batch = len(o.pending)
	}
	claimed := o.pending[:batch]
	o.pending = o.pending[batch:]
	return append([]PendingEvent(nil), claimed...), nil
}

func (o *InMemoryOutbox) MarkPublished(_ context.Context, eventID string, at time.Time) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.Published[eventID] = at
	return nil
}

func (o *InMemoryOutbox) MarkFailed(_ context.Context, eventID, reason string, availableAt *time.Time) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.Attempts[eventID]++
	if availableAt != nil {
		o.Retried[eventID] = *availableAt
		return nil
	}
	o.Failed[eventID] = reason
	return nil
}

var (
	_ TxWriter = (*InMemoryTxWriter)(nil)
)
