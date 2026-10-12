package store

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/proto"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/outbox"

	"github.com/buidangphuc/team-engagement/internal/events"
)

// OutboxTable is the transactional outbox of engagement facts (go-sdk outbox
// shape, migration 0011). Facts are written in the transaction of the change they
// announce, as full EventEnvelopes; the go-sdk relayer publishes them to
// engagement.events. It replaces the old `outbox` table, which stays in place
// until a later cleanup change drops it.
const OutboxTable = "engagement_outbox_events"

// Aggregate types of the facts: what the Kafka key identifies.
const (
	aggregateListing = "Listing"
	aggregateSeller  = "Seller"
)

// factWriter enqueues engagement facts into the outbox inside the caller's
// transaction.
type factWriter struct{ outbox *outbox.Store }

func newFactWriter(pool *pgxpool.Pool) factWriter {
	return factWriter{outbox: outbox.NewStore(pool, OutboxTable)}
}

// enqueue writes one fact row. The envelope principal is the caller, taken from
// the principal the auth interceptor put on ctx; the type is the payload's full
// proto name.
func (w factWriter) enqueue(ctx context.Context, tx pgx.Tx, aggregateType, key string, msg proto.Message) error {
	row, err := events.FactRow(ctx, aggregateType, key, msg)
	if err != nil {
		return err
	}
	return w.outbox.EnqueueTx(ctx, tx, row)
}
