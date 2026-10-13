package store

import (
	"sync"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/outbox"
)

// OutboxTable is the chat.events outbox table (migration 0003). The SDK
// outbox.Store writes it and the SDK outbox.Relayer publishes it.
const OutboxTable = "chat_outbox_events"

// OutboxRow is the enqueue input: one pending chat event persisted in the SAME
// transaction as the message write. Payload is the fully-marshalled
// platform.events.v1.EventEnvelope (produced verbatim by the relayer), EventID
// is that envelope's event_id (the stable dedupe key consumers use) and
// AggregateID is the thread id, the Kafka partition key.
type OutboxRow = outbox.Row

// InMemoryOutbox records enqueued rows for the in-memory chat repository, a
// deterministic fake for unit tests. Nothing is relayed from it.
type InMemoryOutbox struct {
	mu   sync.Mutex
	rows []OutboxRow
}

// NewInMemoryOutbox constructs an empty in-memory outbox.
func NewInMemoryOutbox() *InMemoryOutbox { return &InMemoryOutbox{} }

// Enqueue records row.
func (o *InMemoryOutbox) Enqueue(row OutboxRow) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.rows = append(o.rows, row)
}

// EnqueuedRows returns a copy of the recorded rows in enqueue order.
func (o *InMemoryOutbox) EnqueuedRows() []OutboxRow {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]OutboxRow(nil), o.rows...)
}
