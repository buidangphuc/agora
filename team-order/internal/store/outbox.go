package store

import (
	"context"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/outbox"
)

// OutboxTable is the transactional outbox of order events (ADR-0002, ADR-0013).
// Rows are written in the transaction of the change they announce through the
// go-sdk outbox store; the go-sdk relayer publishes them to order.events.
const OutboxTable = "order_outbox_events"

// mustQueue returns the go-sdk outbox store over OutboxTable. outbox.NewStore
// fails only for a nil pool or an invalid table name, and OutboxTable is a
// constant, so a failure is a programming error at construction, like
// regexp.MustCompile.
func mustQueue(pool *pgxpool.Pool) *outbox.Store {
	q, err := outbox.NewStore(pool, OutboxTable)
	if err != nil {
		panic("store: outbox store: " + err.Error())
	}
	return q
}

// OutboxRow is one order event to persist in the SAME transaction as the write it
// announces. Payload is the fully marshalled platform.events.v1.EventEnvelope
// (published verbatim), AggregateID the order id (the Kafka partition key) and
// EventID the envelope's event_id, the stable key consumers dedupe on.
type OutboxRow = outbox.Row

// OutboxWriter is where the in-memory repositories write their outbox rows.
type OutboxWriter interface {
	Enqueue(ctx context.Context, row OutboxRow) error
}

// InMemoryOutboxRepository is a deterministic fake for unit tests and the
// in-memory (ENV=local) repositories.
type InMemoryOutboxRepository struct {
	mu   sync.Mutex
	rows []OutboxRow
}

// NewInMemoryOutboxRepository constructs an in-memory outbox store.
func NewInMemoryOutboxRepository() *InMemoryOutboxRepository {
	return &InMemoryOutboxRepository{}
}

// Enqueue records row.
func (r *InMemoryOutboxRepository) Enqueue(_ context.Context, row OutboxRow) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rows = append(r.rows, row)
	return nil
}

// EnqueuedRows returns a copy of the recorded rows, oldest first.
func (r *InMemoryOutboxRepository) EnqueuedRows() []OutboxRow {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := make([]OutboxRow, len(r.rows))
	copy(cp, r.rows)
	return cp
}
