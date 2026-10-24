package storetest

import (
	"context"
	"errors"
	"github.com/buidangphuc/team-domain/internal/store"
	"sync"
	"time"
)

type PendingEvent struct {
	EventID     string
	AggregateID string
	Payload     []byte
	Attempts    int
}

type InMemoryTxWriter struct {
	repo      *InMemoryListingRepository
	FailWrite bool

	mu   sync.Mutex
	rows []store.OutboxRow

	heldPrior map[string]string
}

func NewInMemoryTxWriter(repo *InMemoryListingRepository) *InMemoryTxWriter {
	return &InMemoryTxWriter{repo: repo}
}

func (w *InMemoryTxWriter) CreateTx(ctx context.Context, l store.Listing, ev store.EnqueueFn) (store.Listing, error) {
	if w.FailWrite {
		return store.Listing{}, errors.New("simulated write failure")
	}
	out, err := w.repo.Create(ctx, l)
	if err != nil {
		return store.Listing{}, err
	}
	return w.enqueue(out, ev)
}

func (w *InMemoryTxWriter) UpdateTx(ctx context.Context, l store.Listing, ev store.EnqueueFn) (store.Listing, error) {
	if w.FailWrite {
		return store.Listing{}, errors.New("simulated write failure")
	}
	out, err := w.repo.Update(ctx, l)
	if err != nil {
		return store.Listing{}, err
	}
	return w.enqueue(out, ev)
}

func (w *InMemoryTxWriter) DeleteTx(ctx context.Context, id string, ev store.EnqueueFn) (store.Listing, error) {
	if w.FailWrite {
		return store.Listing{}, errors.New("simulated write failure")
	}
	out, err := w.repo.Delete(ctx, id)
	if err != nil {
		return store.Listing{}, err
	}
	return w.enqueue(out, ev)
}
func (w *InMemoryTxWriter) enqueue(out store.Listing, ev store.EnqueueFn) (store.Listing, error) {
	row, err := ev(out)
	if err != nil {
		return store.Listing{}, err
	}
	w.mu.Lock()
	w.rows = append(w.rows, row)
	w.mu.Unlock()
	return out, nil
}
func (w *InMemoryTxWriter) Rows() []store.OutboxRow {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make([]store.OutboxRow, len(w.rows))
	copy(out, w.rows)
	return out
}

type InMemoryOutbox struct {
	mu        sync.Mutex
	pending   []PendingEvent
	Published map[string]time.Time
	Failed    map[string]string
	Retried   map[string]time.Time
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
	_ store.TxWriter = (*InMemoryTxWriter)(nil)
)
