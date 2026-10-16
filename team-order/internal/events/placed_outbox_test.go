package events

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	eventsv1 "github.com/buidangphuc/team-order/generated/platform/events/v1"
	orderv1 "github.com/buidangphuc/team-order/generated/platform/order/v1"
	paymentv1 "github.com/buidangphuc/team-order/generated/platform/payment/v1"
)

func decodePlaced(t *testing.T, payload []byte) (*eventsv1.EventEnvelope, *orderv1.OrderPlaced) {
	t.Helper()
	var env eventsv1.EventEnvelope
	require.NoError(t, proto.Unmarshal(payload, &env))
	var ev orderv1.OrderPlaced
	require.NoError(t, proto.Unmarshal(env.GetPayload(), &ev))
	return &env, &ev
}

// Scenario: Order expiry: Online-pay order gets a 24-hour deadline (the OrderPlaced
// payload carries the order's own payment_due_at).
func TestPlacedEnvelopeMatchesContract(t *testing.T) {
	placedAt := time.Date(2026, 10, 8, 9, 30, 0, 0, time.UTC)
	due := placedAt.Add(24 * time.Hour)
	order := sampleOrder()
	order.CreatedAt = placedAt
	order.UpdatedAt = placedAt
	order.PaymentMethod = int32(paymentv1.PaymentMethod_PAYMENT_METHOD_MOCK_MOMO)
	order.PaymentDueAt = &due

	row, err := BuildPlacedOutboxRow(order)
	require.NoError(t, err)
	assert.Equal(t, "order-123", row.AggregateID, "key = order id")
	assert.Equal(t, "Order", row.AggregateType)
	assert.Equal(t, OrderPlacedEventType, row.EventType)
	assert.Equal(t, "platform.order.v1.OrderPlaced", OrderPlacedEventType)

	env, ev := decodePlaced(t, row.Payload)
	assert.Equal(t, OrderPlacedEventType, env.GetType())
	assert.Equal(t, row.EventID, env.GetEventId())
	assert.Equal(t, OrderPlacedEventID("order-123"), env.GetEventId(), "event id is stable per order")
	assert.Equal(t, "order-123", ev.GetOrderId())
	assert.Equal(t, "buyer-1", ev.GetBuyerId())
	assert.Equal(t, "seller-1", ev.GetSellerId())
	assert.Equal(t, int64(250000), ev.GetTotalAmount())
	assert.Equal(t, "VND", ev.GetCurrency())
	assert.Equal(t, paymentv1.PaymentMethod_PAYMENT_METHOD_MOCK_MOMO, ev.GetPaymentMethod())
	assert.True(t, placedAt.Equal(ev.GetPlacedAt().AsTime()))
	require.NotNil(t, ev.GetPaymentDueAt())
	assert.True(t, due.Equal(ev.GetPaymentDueAt().AsTime()))
	assert.Nil(t, ev.GetShipBy(), "ship_by is left unset by this change")
}

// Scenario: Order expiry: COD order has no deadline.
func TestPlacedEnvelopeCodHasNoPaymentDueAt(t *testing.T) {
	order := sampleOrder()
	order.CreatedAt = time.Date(2026, 10, 8, 9, 30, 0, 0, time.UTC)
	order.PaymentMethod = int32(paymentv1.PaymentMethod_PAYMENT_METHOD_COD)
	row, err := BuildPlacedOutboxRow(order)
	require.NoError(t, err)
	_, ev := decodePlaced(t, row.Payload)
	assert.Nil(t, ev.GetPaymentDueAt())
	assert.Equal(t, paymentv1.PaymentMethod_PAYMENT_METHOD_COD, ev.GetPaymentMethod())
}

// Scenario: Order expiry: Each seller order of one checkout gets its own OrderPlaced
// (distinct event ids per order, so rows never collapse onto one another).
func TestPlacedEventIDIsPerOrderAndDistinctFromOtherFacts(t *testing.T) {
	assert.NotEqual(t, OrderPlacedEventID("a"), OrderPlacedEventID("b"))
	assert.Equal(t, OrderPlacedEventID("a"), OrderPlacedEventID("a"))
	assert.NotEqual(t, OrderPlacedEventID("a"), OrderCancelledEventID("a"))
	assert.NotEqual(t, OrderPlacedEventID("a"), OrderPaidEventID("a"))
}
