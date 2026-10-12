package events_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	sdkconfig "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/outbox"
	sdkpgx "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/pgx"

	eventsv1 "github.com/buidangphuc/team-order/generated/platform/events/v1"
	orderv1 "github.com/buidangphuc/team-order/generated/platform/order/v1"
	"github.com/buidangphuc/team-order/internal/events"
	"github.com/buidangphuc/team-order/internal/store"
)

type sent struct {
	topic string
	key   string
	value []byte
}

// recorder is an outbox.Publisher that records what the relayer produced.
type recorder struct {
	mu   sync.Mutex
	sent []sent
	err  error
}

func (r *recorder) Publish(_ context.Context, topic string, key, value []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err != nil {
		return r.err
	}
	r.sent = append(r.sent, sent{topic, string(key), value})
	return nil
}

func (r *recorder) all() []sent {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]sent(nil), r.sent...)
}

func newRelayer(pool *pgxpool.Pool, pub outbox.Publisher, cfg sdkconfig.Outbox, base time.Duration) *outbox.Relayer {
	return outbox.NewRelayer(outbox.NewStore(pool, store.OutboxTable), pub, outbox.RelayerOptions{
		Topic: events.OrderEventsTopic, Config: cfg, BaseBackoff: base, MaxBackoff: base,
	})
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
	pub := &recorder{}
	relayer := newRelayer(pool, pub, sdkconfig.Outbox{BatchSize: 10}, 0)

	created, err := orders.CreateOrder(ctx, sampleOrder())
	require.NoError(t, err)
	_, err = orders.UpdateOrderStatusFrom(ctx, created.ID, store.OrderStatusPaid, pendingOnly, "")
	require.NoError(t, err)

	rep, err := relayer.RelayOnce(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, rep.Published)
	got := pub.all()
	require.Len(t, got, 1)
	assert.Equal(t, events.OrderEventsTopic, got[0].topic)
	assert.Equal(t, created.ID, got[0].key)
	var stored []byte
	require.NoError(t, pool.QueryRow(ctx, `SELECT payload FROM order_outbox_events WHERE aggregate_id = $1`, created.ID).Scan(&stored))
	assert.Equal(t, stored, got[0].value)

	rep, err = relayer.RelayOnce(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, rep.Published, "published rows are not re-sent")
	assert.Len(t, pub.all(), 1)
}

func TestRelayerRetriesThenParks(t *testing.T) {
	ctx := context.Background()
	pool := orderDB(t)
	require.NoError(t, sdkpgx.WithTx(ctx, pool, func(tx pgx.Tx) error {
		return outbox.NewStore(pool, store.OutboxTable).EnqueueTx(ctx, tx, store.OutboxRow{
			EventID: "e1", AggregateType: "Order", AggregateID: "o1", EventType: "t", Payload: []byte("x"),
		})
	}))
	pub := &recorder{err: errors.New("broker down")}
	relayer := newRelayer(pool, pub, sdkconfig.Outbox{MaxAttempts: 3}, time.Millisecond)

	for i := 0; i < 3; i++ {
		time.Sleep(20 * time.Millisecond) // let the backoff gate open
		rep, err := relayer.RelayOnce(ctx)
		require.NoError(t, err)
		assert.Equal(t, 0, rep.Published)
		assert.Equal(t, 1, rep.Claimed)
	}
	// Parked after MaxAttempts: no longer claimed.
	time.Sleep(20 * time.Millisecond)
	rep, err := relayer.RelayOnce(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, rep.Claimed)
	var status string
	require.NoError(t, pool.QueryRow(ctx, `SELECT status FROM order_outbox_events WHERE event_id = 'e1'`).Scan(&status))
	assert.Equal(t, "failed", status)
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
	for _, r := range []struct {
		id  string
		age time.Duration
	}{{"evt-updated", 2 * time.Second}, {"evt-created", 0}} {
		_, err := pool.Exec(ctx, `INSERT INTO order_outbox_events
			(event_id, aggregate_type, aggregate_id, event_type, payload, request_id, status, attempts, available_at, created_at)
			VALUES ($1, 'Agg', 'agg-1', 'T', $2, '', 'pending', 0, $3, $3)`, r.id, []byte(r.id), base.Add(r.age))
		require.NoError(t, err)
	}

	pub := &recorder{}
	_, err := newRelayer(pool, pub, sdkconfig.Outbox{BatchSize: 10}, 0).RelayOnce(ctx)
	require.NoError(t, err)
	got := pub.all()
	require.Len(t, got, 2)
	assert.Equal(t, "evt-created", string(got[0].value))
	assert.Equal(t, "evt-updated", string(got[1].value))
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

	pub := &recorder{}
	rep, err := newRelayer(pool, pub, sdkconfig.Outbox{BatchSize: 10}, 0).RelayOnce(ctx)
	require.NoError(t, err)
	require.Equal(t, 2, rep.Published, "one OrderPaidEvent + one OrderCancelled, nothing for the lost claim")

	var cancels []sent
	for _, p := range pub.all() {
		var env eventsv1.EventEnvelope
		require.NoError(t, proto.Unmarshal(p.value, &env))
		if env.GetType() == events.OrderCancelledEventType {
			cancels = append(cancels, p)
		}
	}
	require.Len(t, cancels, 1)
	assert.Equal(t, events.OrderEventsTopic, cancels[0].topic)
	assert.Equal(t, created.ID, cancels[0].key)
	var env eventsv1.EventEnvelope
	require.NoError(t, proto.Unmarshal(cancels[0].value, &env))
	var ev orderv1.OrderCancelled
	require.NoError(t, proto.Unmarshal(env.GetPayload(), &ev))
	assert.Equal(t, orderv1.OrderStatus_ORDER_STATUS_PAID, ev.GetPreviousStatus())
}
