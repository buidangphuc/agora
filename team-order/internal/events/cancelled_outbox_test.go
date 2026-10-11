package events

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	eventsv1 "github.com/buidangphuc/team-order/generated/platform/events/v1"
	orderv1 "github.com/buidangphuc/team-order/generated/platform/order/v1"
	"github.com/buidangphuc/team-order/internal/repository"
)

var cancelFrom = []repository.OrderStatus{repository.OrderStatusPending, repository.OrderStatusPaid}

func decodeCancelled(t *testing.T, payload []byte) (*eventsv1.EventEnvelope, *orderv1.OrderCancelled) {
	t.Helper()
	var env eventsv1.EventEnvelope
	require.NoError(t, proto.Unmarshal(payload, &env))
	var ev orderv1.OrderCancelled
	require.NoError(t, proto.Unmarshal(env.GetPayload(), &ev))
	return &env, &ev
}

// The payload matches the order.proto contract: an EventEnvelope of type
// platform.order.v1.OrderCancelled, stable event id per order, previous_status
// PAID iff paid_at is set.
func TestCancelledEnvelopeMatchesContract(t *testing.T) {
	cancelledAt := time.Date(2026, 10, 8, 9, 30, 0, 0, time.UTC)
	for _, tc := range []struct {
		name   string
		paidAt *time.Time
		want   orderv1.OrderStatus
	}{
		{"from pending", nil, orderv1.OrderStatus_ORDER_STATUS_PENDING},
		{"from paid", &cancelledAt, orderv1.OrderStatus_ORDER_STATUS_PAID},
	} {
		t.Run(tc.name, func(t *testing.T) {
			order := sampleOrder()
			order.Status = repository.OrderStatusCancelled
			order.UpdatedAt = cancelledAt
			order.PaidAt = tc.paidAt

			row, err := BuildCancelledOutboxRow(order)
			require.NoError(t, err)
			assert.Equal(t, "order-123", row.AggregateID, "key = order id")
			assert.Equal(t, "Order", row.AggregateType)
			assert.Equal(t, OrderCancelledEventType, row.EventType)

			env, ev := decodeCancelled(t, row.Payload)
			assert.Equal(t, "platform.order.v1.OrderCancelled", env.GetType())
			assert.Equal(t, row.EventID, env.GetEventId())
			assert.Equal(t, OrderCancelledEventID("order-123"), env.GetEventId(), "event id is stable per order")
			assert.True(t, cancelledAt.Equal(env.GetOccurredAt().AsTime()))

			assert.Equal(t, "order-123", ev.GetOrderId())
			assert.Equal(t, "buyer-1", ev.GetBuyerId())
			assert.Equal(t, "seller-1", ev.GetSellerId())
			assert.Equal(t, tc.want, ev.GetPreviousStatus())
			assert.Equal(t, int64(250000), ev.GetTotalAmount())
			assert.Equal(t, "VND", ev.GetCurrency())
			assert.True(t, cancelledAt.Equal(ev.GetCancelledAt().AsTime()))
		})
	}
	assert.NotEqual(t, OrderCancelledEventID("order-123"), OrderPaidEventID("order-123"),
		"the cancel fact and the paid fact of one order never share an id")
	assert.NotEqual(t, OrderCancelledEventID("order-123"), OrderCancelledEventID("order-456"))
}

// The won claim to CANCELLED writes the row through the wired builder and the
// relayer publishes it to order.events keyed by order id.
func TestCancelledFactIsRelayedToOrderEvents(t *testing.T) {
	ctx := context.Background()
	outbox := repository.NewInMemoryOutboxRepository()
	orders := repository.NewInMemoryOrderRepository(
		repository.WithPaidOutbox(BuildPaidOutboxRow),
		repository.WithCancelledOutbox(BuildCancelledOutboxRow),
		repository.WithInMemoryOutbox(outbox),
	)
	created, err := orders.CreateOrder(ctx, sampleOrder())
	require.NoError(t, err)
	_, err = orders.UpdateOrderStatusFrom(ctx, created.ID, repository.OrderStatusPaid, pendingOnly, "")
	require.NoError(t, err)
	_, err = orders.UpdateOrderStatusFrom(ctx, created.ID, repository.OrderStatusCancelled, cancelFrom, "")
	require.NoError(t, err)
	_, err = orders.UpdateOrderStatusFrom(ctx, created.ID, repository.OrderStatusCancelled, cancelFrom, "")
	require.ErrorIs(t, err, repository.ErrStatusConflict, "a lost claim")

	pub := &mockPublisher{}
	relayer := NewRelayer(outbox, pub, RelayerConfig{BatchSize: 10}, slog.Default())
	n, err := relayer.SweepClaims(ctx)
	require.NoError(t, err)
	require.Equal(t, 2, n, "one OrderPaidEvent + one OrderCancelled, nothing for the lost claim")

	var cancels []published
	for _, p := range pub.published {
		var env eventsv1.EventEnvelope
		require.NoError(t, proto.Unmarshal(p.payload, &env))
		if env.GetType() == OrderCancelledEventType {
			cancels = append(cancels, p)
		}
	}
	require.Len(t, cancels, 1)
	assert.Equal(t, OrderEventsTopic, cancels[0].topic)
	assert.Equal(t, created.ID, cancels[0].key)
	_, ev := decodeCancelled(t, cancels[0].payload)
	assert.Equal(t, orderv1.OrderStatus_ORDER_STATUS_PAID, ev.GetPreviousStatus())
}
