package events

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	eventsv1 "github.com/buidangphuc/team-order/generated/platform/events/v1"
	orderv1 "github.com/buidangphuc/team-order/generated/platform/order/v1"
	"github.com/buidangphuc/team-order/internal/repository"
)

type published struct {
	topic   string
	key     string
	payload []byte
}

type mockPublisher struct {
	mu        sync.Mutex
	published []published
	err       error
}

func (m *mockPublisher) Publish(_ context.Context, topic, key string, payload []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return m.err
	}
	m.published = append(m.published, published{topic, key, payload})
	return nil
}

func sampleOrder() repository.Order {
	return repository.Order{
		ID:          "order-123",
		BuyerID:     "buyer-1",
		SellerID:    "seller-1",
		Status:      repository.OrderStatusPending,
		TotalAmount: 250000,
		Currency:    "VND",
		Items: []repository.OrderItem{
			{ID: "item-1", ListingID: "listing-1", VariantID: "var-1", Quantity: 2, UnitPrice: 100000},
			{ID: "item-2", ListingID: "listing-2", Quantity: 1, UnitPrice: 50000},
		},
	}
}

func newRepos() (*repository.InMemoryOrderRepository, *repository.InMemoryOutboxRepository) {
	outbox := repository.NewInMemoryOutboxRepository()
	orders := repository.NewInMemoryOrderRepository(
		repository.WithPaidOutbox(BuildPaidOutboxRow),
		repository.WithInMemoryOutbox(outbox),
	)
	return orders, outbox
}

// The outbox row is written together with the PAID transition, only for the
// first transition to PAID, and not for other states.
func TestPaidTransitionWritesOutboxRow(t *testing.T) {
	ctx := context.Background()
	orders, outbox := newRepos()
	created, err := orders.CreateOrder(ctx, sampleOrder())
	require.NoError(t, err)
	assert.Empty(t, outbox.EnqueuedRows(), "PENDING must not emit")

	_, err = orders.UpdateOrderStatus(ctx, created.ID, repository.OrderStatusPaid, "")
	require.NoError(t, err)
	rows := outbox.EnqueuedRows()
	require.Len(t, rows, 1)
	assert.Equal(t, created.ID, rows[0].AggregateID)
	assert.Equal(t, OrderPaidEventType, rows[0].EventType)
	assert.Equal(t, "Order", rows[0].AggregateType)

	// Re-applying PAID, and later states, do not emit another PAID fact.
	_, err = orders.UpdateOrderStatus(ctx, created.ID, repository.OrderStatusPaid, "")
	require.NoError(t, err)
	_, err = orders.UpdateOrderStatus(ctx, created.ID, repository.OrderStatusShipped, "TRK1")
	require.NoError(t, err)
	assert.Len(t, outbox.EnqueuedRows(), 1)
}

type failingOutbox struct{ repository.OutboxRepository }

func (failingOutbox) Enqueue(context.Context, repository.OutboxRow) error {
	return errors.New("outbox write failed")
}

// If the outbox row cannot be written the status change does not happen either
// (the in-memory analogue of the transaction rolling back).
func TestPaidTransitionRollsBackWhenOutboxFails(t *testing.T) {
	ctx := context.Background()

	t.Run("enqueue error", func(t *testing.T) {
		orders := repository.NewInMemoryOrderRepository(
			repository.WithPaidOutbox(BuildPaidOutboxRow),
			repository.WithInMemoryOutbox(failingOutbox{repository.NewInMemoryOutboxRepository()}),
		)
		created, err := orders.CreateOrder(ctx, sampleOrder())
		require.NoError(t, err)
		_, err = orders.UpdateOrderStatus(ctx, created.ID, repository.OrderStatusPaid, "")
		require.Error(t, err)
		got, err := orders.GetOrder(ctx, created.ID)
		require.NoError(t, err)
		assert.Equal(t, repository.OrderStatusPending, got.Status)
	})

	t.Run("build error", func(t *testing.T) {
		outbox := repository.NewInMemoryOutboxRepository()
		orders := repository.NewInMemoryOrderRepository(
			repository.WithPaidOutbox(func(repository.Order) (repository.OutboxRow, error) {
				return repository.OutboxRow{}, errors.New("boom")
			}),
			repository.WithInMemoryOutbox(outbox),
		)
		created, err := orders.CreateOrder(ctx, sampleOrder())
		require.NoError(t, err)
		_, err = orders.UpdateOrderStatus(ctx, created.ID, repository.OrderStatusPaid, "")
		require.Error(t, err)
		got, _ := orders.GetOrder(ctx, created.ID)
		assert.Equal(t, repository.OrderStatusPending, got.Status)
		assert.Empty(t, outbox.EnqueuedRows())
	})
}

// The relayer publishes the stored envelope verbatim to the topic keyed by
// order_id, then marks the row published so it is not sent twice.
func TestRelayerPublishesAndMarksPublished(t *testing.T) {
	ctx := context.Background()
	orders, outbox := newRepos()
	pub := &mockPublisher{}
	relayer := NewRelayer(outbox, pub, RelayerConfig{BatchSize: 10}, slog.Default())

	created, err := orders.CreateOrder(ctx, sampleOrder())
	require.NoError(t, err)
	_, err = orders.UpdateOrderStatus(ctx, created.ID, repository.OrderStatusPaid, "")
	require.NoError(t, err)

	n, err := relayer.SweepClaims(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	require.Len(t, pub.published, 1)
	assert.Equal(t, OrderEventsTopic, pub.published[0].topic)
	assert.Equal(t, "order-123", pub.published[0].key)
	assert.Equal(t, outbox.EnqueuedRows()[0].Payload, pub.published[0].payload)

	n, err = relayer.SweepClaims(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, n, "published rows are not re-sent")
	assert.Len(t, pub.published, 1)
}

func TestRelayerRetriesThenParks(t *testing.T) {
	ctx := context.Background()
	outbox := repository.NewInMemoryOutboxRepository()
	require.NoError(t, outbox.Enqueue(ctx, repository.OutboxRow{EventID: "e1", AggregateID: "o1", Payload: []byte("x")}))
	pub := &mockPublisher{err: errors.New("broker down")}
	relayer := NewRelayer(outbox, pub, RelayerConfig{MaxAttempts: 3}, slog.Default())

	for i := 0; i < 3; i++ {
		n, err := relayer.SweepClaims(ctx)
		require.NoError(t, err)
		assert.Equal(t, 0, n)
	}
	// Parked after MaxAttempts: no longer claimed.
	pending, err := outbox.ClaimPending(ctx, 10, time.Second)
	require.NoError(t, err)
	assert.Empty(t, pending)

	// Backoff doubles and is capped.
	cfg := RelayerConfig{}.withDefaults()
	assert.Equal(t, time.Second, cfg.backoff(1))
	assert.Equal(t, 4*time.Second, cfg.backoff(3))
	assert.Equal(t, cfg.MaxBackoff, cfg.backoff(30))
}

// The payload matches the ADR-0013 / order.proto contract: an EventEnvelope of
// type platform.order.v1.OrderPaidEvent carrying order_id and one line-item fact
// per order line.
func TestPaidEnvelopeMatchesContract(t *testing.T) {
	order := sampleOrder()
	order.Status = repository.OrderStatusPaid
	order.UpdatedAt = time.Date(2026, 10, 5, 9, 30, 0, 0, time.UTC)

	row, err := BuildPaidOutboxRow(order)
	require.NoError(t, err)
	assert.Equal(t, "order-123", row.AggregateID)

	var env eventsv1.EventEnvelope
	require.NoError(t, proto.Unmarshal(row.Payload, &env))
	assert.Equal(t, "platform.order.v1.OrderPaidEvent", env.GetType())
	assert.Equal(t, row.EventID, env.GetEventId())
	assert.Equal(t, OrderPaidEventID("order-123"), env.GetEventId(), "event id is stable per order")
	assert.True(t, order.UpdatedAt.Equal(env.GetOccurredAt().AsTime()))

	var ev orderv1.OrderPaidEvent
	require.NoError(t, proto.Unmarshal(env.GetPayload(), &ev))
	assert.Equal(t, "order-123", ev.GetOrderId())
	assert.Equal(t, "buyer-1", ev.GetBuyerId())
	assert.Equal(t, int64(250000), ev.GetTotalAmount())
	assert.Equal(t, "VND", ev.GetCurrency())
	assert.True(t, order.UpdatedAt.Equal(ev.GetPaidAt().AsTime()))
	require.Len(t, ev.GetItems(), 2)
	first := ev.GetItems()[0]
	assert.Equal(t, "listing-1", first.GetListingId())
	assert.Equal(t, "var-1", first.GetVariantId())
	assert.Equal(t, "seller-1", first.GetSellerId())
	assert.Equal(t, int32(2), first.GetQuantity())
	assert.Equal(t, int64(100000), first.GetUnitPrice())
	assert.Equal(t, "VND", first.GetCurrency())
}
