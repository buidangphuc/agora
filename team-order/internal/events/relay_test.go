package events_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kgo"
	"google.golang.org/protobuf/proto"

	sdkconfig "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/postgres"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/testkit"

	eventsv1 "github.com/buidangphuc/team-order/generated/platform/events/v1"
	orderv1 "github.com/buidangphuc/team-order/generated/platform/order/v1"
	"github.com/buidangphuc/team-order/internal/events"
	"github.com/buidangphuc/team-order/internal/store"
)

func newRelayer(t *testing.T, pool *pgxpool.Pool, pub outbox.Publisher, cfg sdkconfig.Outbox, base time.Duration) *outbox.Relayer {
	t.Helper()
	queue, err := outbox.NewStore(pool, store.OutboxTable)
	require.NoError(t, err)
	relayer, err := outbox.NewRelayer(queue, pub, outbox.RelayerOptions{
		Topic: events.OrderEventsTopic, Config: cfg, BaseBackoff: base, MaxBackoff: base,
	})
	require.NoError(t, err)
	return relayer
}

// permanentPublisher accepts every record but the one with key bad, which it fails
// with an error no retry can fix.
type permanentPublisher struct{ bad string }

func (p permanentPublisher) PublishRecord(_ context.Context, r *kgo.Record) error {
	if string(r.Key) == p.bad {
		return outbox.Permanent(errors.New("record rejected by the broker"))
	}
	return nil
}

func sampleOrder() store.Order {
	return store.Order{
		ID: "order-123", BuyerID: "buyer-1", SellerID: "seller-1", Status: store.OrderStatusPending,
		TotalAmount: 250000, Currency: "VND",
		Items: []store.OrderItem{{ID: "item-1", ListingID: "listing-1", Quantity: 2, UnitPrice: 100000}},
	}
}

var (
	pendingOnly = []store.OrderStatus{store.OrderStatusPending}
	cancelFrom  = []store.OrderStatus{store.OrderStatusPending, store.OrderStatusPaid}
)

// The relayer publishes the stored envelope verbatim to the topic keyed by
// order_id, then marks the row published so it is not sent twice.
func TestRelayerPublishesAndMarksPublished(t *testing.T) {
	ctx := context.Background()
	pool := orderDB(t)
	orders := store.NewPostgresOrderRepository(pool, store.WithPaidOutbox(events.BuildPaidOutboxRow))
	pub := testkit.NewRecorder()
	relayer := newRelayer(t, pool, pub, sdkconfig.Outbox{BatchSize: 10}, 0)

	created, err := orders.CreateOrder(ctx, sampleOrder())
	require.NoError(t, err)
	_, err = orders.UpdateOrderStatusFrom(ctx, created.ID, store.OrderStatusPaid, pendingOnly, "")
	require.NoError(t, err)

	rep, err := relayer.RelayOnce(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, rep.Published)
	got := pub.Records()
	require.Len(t, got, 1)
	assert.Equal(t, events.OrderEventsTopic, got[0].Topic)
	assert.Equal(t, created.ID, string(got[0].Key))
	var stored []byte
	require.NoError(t, pool.QueryRow(ctx, `SELECT payload FROM order_outbox_events WHERE aggregate_id = $1`, created.ID).Scan(&stored))
	assert.Equal(t, stored, got[0].Value)

	rep, err = relayer.RelayOnce(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, rep.Published, "published rows are not re-sent")
	assert.Len(t, pub.Records(), 1)
}

// go-sdk v1 retries every transient publish error forever with capped backoff
// (OUTBOX_MAX_ATTEMPTS is gone), so a broker outage never parks a row; only an
// error that is permanent for the record parks it, at once.
func TestRelayerRetriesThenParks(t *testing.T) {
	ctx := context.Background()
	pool := orderDB(t)
	queue, err := outbox.NewStore(pool, store.OutboxTable)
	require.NoError(t, err)
	enqueue := func(id string) {
		require.NoError(t, postgres.WithTx(ctx, pool, func(tx pgx.Tx) error {
			return queue.EnqueueTx(ctx, tx, store.OutboxRow{
				EventID: id, AggregateType: "Order", AggregateID: id, EventType: "t", Payload: []byte("x"),
			})
		}))
	}
	statusOf := func(id string) string {
		var status string
		require.NoError(t, pool.QueryRow(ctx, `SELECT status FROM order_outbox_events WHERE event_id = $1`, id).Scan(&status))
		return status
	}

	// A broker outage: claimed on every pass, never published, never parked.
	enqueue("e1")
	down := testkit.NewRecorder()
	down.FailOn("e1")
	relayer := newRelayer(t, pool, down, sdkconfig.Outbox{}, time.Millisecond)
	for i := 0; i < 4; i++ {
		time.Sleep(20 * time.Millisecond) // let the backoff gate open
		rep, err := relayer.RelayOnce(ctx)
		require.NoError(t, err)
		assert.Equal(t, 0, rep.Published)
		assert.Equal(t, 1, rep.Claimed)
	}
	assert.Equal(t, "pending", statusOf("e1"), "a transient publish error must not park the row")

	// A record the broker can never accept parks at once and is no longer claimed.
	enqueue("e2")
	time.Sleep(20 * time.Millisecond) // let the backoff gate of e1 open
	relayer = newRelayer(t, pool, permanentPublisher{bad: "e2"}, sdkconfig.Outbox{}, time.Millisecond)
	rep, err := relayer.RelayOnce(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, rep.Parked)
	assert.Equal(t, "failed", statusOf("e2"))
	assert.Equal(t, "published", statusOf("e1"), "the broker is back for the other rows")
}

// Regression: UPDATE ... RETURNING does not preserve the claim subquery's ORDER BY,
// so two events for one aggregate in the same batch could be relayed newest-first
// (a consumer diffing snapshots then misses a price drop). Rows are inserted so the
// physical order is the reverse of created_at; the claim must still relay them
// oldest-first.
func TestClaimKeepsCreationOrder(t *testing.T) {
	ctx := context.Background()
	pool := orderDB(t)
	base := time.Now().Add(-time.Minute)
	// Inserted newer-first so heap order disagrees with created_at.
	// seq is the creation order; created_at only ties inside one transaction.
	for _, r := range []struct {
		id  string
		age time.Duration
		seq int64
	}{{"evt-updated", 2 * time.Second, 2}, {"evt-created", 0, 1}} {
		_, err := pool.Exec(ctx, `INSERT INTO order_outbox_events
			(event_id, aggregate_type, aggregate_id, event_type, payload, request_id, seq, status, attempts, available_at, created_at)
			VALUES ($1, 'Agg', 'agg-1', 'T', $2, '', $4, 'pending', 0, $3, $3)`, r.id, []byte(r.id), base.Add(r.age), r.seq)
		require.NoError(t, err)
	}

	pub := testkit.NewRecorder()
	_, err := newRelayer(t, pool, pub, sdkconfig.Outbox{BatchSize: 10}, 0).RelayOnce(ctx)
	require.NoError(t, err)
	got := pub.Records()
	require.Len(t, got, 2)
	assert.Equal(t, "evt-created", string(got[0].Value))
	assert.Equal(t, "evt-updated", string(got[1].Value))
}

// The won claim to CANCELLED writes the row through the wired builder and the
// relayer publishes it to order.events keyed by order id.
func TestCancelledFactIsRelayedToOrderEvents(t *testing.T) {
	ctx := context.Background()
	pool := orderDB(t)
	orders := store.NewPostgresOrderRepository(pool,
		store.WithPaidOutbox(events.BuildPaidOutboxRow),
		store.WithCancelledOutbox(events.BuildCancelledOutboxRow),
	)
	created, err := orders.CreateOrder(ctx, sampleOrder())
	require.NoError(t, err)
	_, err = orders.UpdateOrderStatusFrom(ctx, created.ID, store.OrderStatusPaid, pendingOnly, "")
	require.NoError(t, err)
	_, err = orders.UpdateOrderStatusFrom(ctx, created.ID, store.OrderStatusCancelled, cancelFrom, "")
	require.NoError(t, err)
	_, err = orders.UpdateOrderStatusFrom(ctx, created.ID, store.OrderStatusCancelled, cancelFrom, "")
	require.ErrorIs(t, err, store.ErrStatusConflict, "a lost claim")

	pub := testkit.NewRecorder()
	rep, err := newRelayer(t, pool, pub, sdkconfig.Outbox{BatchSize: 10}, 0).RelayOnce(ctx)
	require.NoError(t, err)
	require.Equal(t, 2, rep.Published, "one OrderPaidEvent + one OrderCancelled, nothing for the lost claim")

	var cancels []testkit.Record
	for _, p := range pub.Records() {
		var env eventsv1.EventEnvelope
		require.NoError(t, proto.Unmarshal(p.Value, &env))
		if env.GetType() == events.OrderCancelledEventType {
			cancels = append(cancels, p)
		}
	}
	require.Len(t, cancels, 1)
	assert.Equal(t, events.OrderEventsTopic, cancels[0].Topic)
	assert.Equal(t, created.ID, string(cancels[0].Key))
	var env eventsv1.EventEnvelope
	require.NoError(t, proto.Unmarshal(cancels[0].Value, &env))
	var ev orderv1.OrderCancelled
	require.NoError(t, proto.Unmarshal(env.GetPayload(), &ev))
	assert.Equal(t, orderv1.OrderStatus_ORDER_STATUS_PAID, ev.GetPreviousStatus())
}
