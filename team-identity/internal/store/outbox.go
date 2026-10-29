package repository

import (
	"context"
	"sync"
	"time"
)

// OutboxRow is the enqueue input: one pending event to persist in the SAME
// transaction as the business write. Payload is the fully-marshalled
// platform.events.v1.EventEnvelope (produced verbatim by the relayer), and
// EventID is that envelope's event_id — the stable dedupe key consumers use.
type OutboxRow struct {
	EventID       string
	AggregateType string // 'Session'
	AggregateID   string // user_id; the Kafka partition key
	EventType     string // 'platform.identity.v1.SessionRevoked'
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

// EnqueueFn builds the outbox row for a just-revoked session. The transactional
// repository calls it INSIDE the write transaction, so the revoke and the outbox
// INSERT still commit atomically. Returning an error rolls the revoke back.
type EnqueueFn func(revoked Session) (OutboxRow, error)

// InMemoryOutbox is a fake claim/mark surface for relayer tests. It is
// intentionally simple (no locking semantics) — the FOR UPDATE SKIP LOCKED
// behaviour is only meaningful against real Postgres.
type InMemoryOutbox struct {
	mu        sync.Mutex
	pending   []PendingEvent
	Published map[string]time.Time
	Failed    map[string]string    // event_id -> last error (parked)
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

// ClaimPending returns (and removes) up to batch pending events.
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
