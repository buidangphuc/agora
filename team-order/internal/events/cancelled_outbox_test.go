package events

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	eventsv1 "github.com/buidangphuc/team-order/generated/platform/events/v1"
	orderv1 "github.com/buidangphuc/team-order/generated/platform/order/v1"
	"github.com/buidangphuc/team-order/internal/store"
)

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
			order.Status = store.OrderStatusCancelled
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
