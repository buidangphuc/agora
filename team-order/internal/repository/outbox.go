package repository

import (
	"context"
	"sync"
	"time"
)

// OutboxRow is the enqueue input: one pending order event to persist in the
// SAME transaction as the order-status write. Payload is the fully-marshalled
// platform.events.v1.EventEnvelope (produced verbatim by the relayer), and
// EventID is that envelope's event_id — the stable dedupe key consumers use.
// Mirrors team-payment and team-domain transactional outbox (ADR-0002, ADR-0013).
type OutboxRow struct {
	EventID       string
	AggregateType string // 'Order'
	AggregateID   string // order_id; the Kafka partition key
	EventType     string // e.g. 'platform.order.v1.OrderPaidEvent'
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

// OutboxRepository manages the persistence and claiming of outbox events.
type OutboxRepository interface {
	Enqueue(ctx context.Context, row OutboxRow) error
	ClaimPending(ctx context.Context, limit int, leaseDuration time.Duration) ([]PendingEvent, error)
	MarkPublished(ctx context.Context, eventID string) error
	MarkFailed(ctx context.Context, eventID string, errStr string, retryDelay time.Duration) error
}

// InMemoryOutboxRepository is a deterministic fake for unit tests.
type InMemoryOutboxRepository struct {
	mu        sync.Mutex
	rows      []OutboxRow
	published []string
	failed    map[string]string
}

// NewInMemoryOutboxRepository constructs an in-memory outbox store.
func NewInMemoryOutboxRepository() *InMemoryOutboxRepository {
	return &InMemoryOutboxRepository{
		failed: make(map[string]string),
	}
}

func (r *InMemoryOutboxRepository) Enqueue(ctx context.Context, row OutboxRow) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rows = append(r.rows, row)
	return nil
}

func (r *InMemoryOutboxRepository) ClaimPending(ctx context.Context, limit int, leaseDuration time.Duration) ([]PendingEvent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	pubSet := make(map[string]bool)
	for _, id := range r.published {
		pubSet[id] = true
	}

	var pending []PendingEvent
	for _, row := range r.rows {
		if !pubSet[row.EventID] {
			pending = append(pending, PendingEvent{
				EventID:     row.EventID,
				AggregateID: row.AggregateID,
				Payload:     row.Payload,
				Attempts:    0,
			})
			if len(pending) >= limit {
				break
			}
		}
	}
	return pending, nil
}

func (r *InMemoryOutboxRepository) MarkPublished(ctx context.Context, eventID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.published = append(r.published, eventID)
	return nil
}

func (r *InMemoryOutboxRepository) MarkFailed(ctx context.Context, eventID string, errStr string, retryDelay time.Duration) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.failed[eventID] = errStr
	return nil
}

func (r *InMemoryOutboxRepository) EnqueuedRows() []OutboxRow {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := make([]OutboxRow, len(r.rows))
	copy(cp, r.rows)
	return cp
}
