package events

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	eventsv1 "github.com/buidangphuc/team-order/generated/platform/events/v1"
	orderv1 "github.com/buidangphuc/team-order/generated/platform/order/v1"
	"github.com/buidangphuc/team-order/internal/store"
)

func sampleOrder() store.Order {
	return store.Order{
		ID:          "order-123",
		BuyerID:     "buyer-1",
		SellerID:    "seller-1",
		Status:      store.OrderStatusPending,
		TotalAmount: 250000,
		Currency:    "VND",
		Items: []store.OrderItem{
			{ID: "item-1", ListingID: "listing-1", VariantID: "var-1", Quantity: 2, UnitPrice: 100000},
			{ID: "item-2", ListingID: "listing-2", Quantity: 1, UnitPrice: 50000},
		},
	}
}

func newRepos() (*store.InMemoryOrderRepository, *store.InMemoryOutboxRepository) {
	outbox := store.NewInMemoryOutboxRepository()
	orders := store.NewInMemoryOrderRepository(
		store.WithPaidOutbox(BuildPaidOutboxRow),
		store.WithInMemoryOutbox(outbox),
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

	_, err = orders.UpdateOrderStatusFrom(ctx, created.ID, store.OrderStatusPaid, pendingOnly, "")
	require.NoError(t, err)
	rows := outbox.EnqueuedRows()
	require.Len(t, rows, 1)
	assert.Equal(t, created.ID, rows[0].AggregateID)
	assert.Equal(t, OrderPaidEventType, rows[0].EventType)
	assert.Equal(t, "Order", rows[0].AggregateType)

	// Re-applying PAID conflicts (no longer Pending) and later states do not emit
	// another PAID fact.
	_, err = orders.UpdateOrderStatusFrom(ctx, created.ID, store.OrderStatusPaid, pendingOnly, "")
	require.ErrorIs(t, err, store.ErrStatusConflict)
	_, err = orders.UpdateOrderStatusFrom(ctx, created.ID, store.OrderStatusShipped, []store.OrderStatus{store.OrderStatusPaid}, "TRK1")
	require.NoError(t, err)
	assert.Len(t, outbox.EnqueuedRows(), 1)
}

var pendingOnly = []store.OrderStatus{store.OrderStatusPending}

type failingOutbox struct{ store.OutboxWriter }

func (failingOutbox) Enqueue(context.Context, store.OutboxRow) error {
	return errors.New("outbox write failed")
}

// If the outbox row cannot be written the status change does not happen either
// (the in-memory analogue of the transaction rolling back).
func TestPaidTransitionRollsBackWhenOutboxFails(t *testing.T) {
	ctx := context.Background()

	t.Run("enqueue error", func(t *testing.T) {
		orders := store.NewInMemoryOrderRepository(
			store.WithPaidOutbox(BuildPaidOutboxRow),
			store.WithInMemoryOutbox(failingOutbox{store.NewInMemoryOutboxRepository()}),
		)
		created, err := orders.CreateOrder(ctx, sampleOrder())
		require.NoError(t, err)
		_, err = orders.UpdateOrderStatusFrom(ctx, created.ID, store.OrderStatusPaid, pendingOnly, "")
		require.Error(t, err)
		got, err := orders.GetOrder(ctx, created.ID)
		require.NoError(t, err)
		assert.Equal(t, store.OrderStatusPending, got.Status)
	})

	t.Run("build error", func(t *testing.T) {
		outbox := store.NewInMemoryOutboxRepository()
		orders := store.NewInMemoryOrderRepository(
			store.WithPaidOutbox(func(store.Order) (store.OutboxRow, error) {
				return store.OutboxRow{}, errors.New("boom")
			}),
			store.WithInMemoryOutbox(outbox),
		)
		created, err := orders.CreateOrder(ctx, sampleOrder())
		require.NoError(t, err)
		_, err = orders.UpdateOrderStatusFrom(ctx, created.ID, store.OrderStatusPaid, pendingOnly, "")
		require.Error(t, err)
		got, _ := orders.GetOrder(ctx, created.ID)
		assert.Equal(t, store.OrderStatusPending, got.Status)
		assert.Empty(t, outbox.EnqueuedRows())
	})
}

// The payload matches the ADR-0013 / order.proto contract: an EventEnvelope of
// type platform.order.v1.OrderPaidEvent carrying order_id and one line-item fact
// per order line.
func TestPaidEnvelopeMatchesContract(t *testing.T) {
	order := sampleOrder()
	order.Status = store.OrderStatusPaid
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
