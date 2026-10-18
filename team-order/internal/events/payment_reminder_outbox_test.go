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

func decodeReminder(t *testing.T, payload []byte) (*eventsv1.EventEnvelope, *orderv1.OrderPaymentReminder) {
	t.Helper()
	var env eventsv1.EventEnvelope
	require.NoError(t, proto.Unmarshal(payload, &env))
	var ev orderv1.OrderPaymentReminder
	require.NoError(t, proto.Unmarshal(env.GetPayload(), &ev))
	return &env, &ev
}

// Scenario: Order expiry: Reminder is emitted two hours before the deadline
// (the OrderPaymentReminder envelope carries the order's own payment_due_at).
func TestPaymentReminderEnvelopeMatchesContract(t *testing.T) {
	due := time.Date(2026, 10, 9, 9, 30, 0, 0, time.UTC)
	sent := due.Add(-2 * time.Hour)
	order := sampleOrder()
	order.PaymentMethod = int32(paymentv1.PaymentMethod_PAYMENT_METHOD_MOCK_MOMO)
	order.PaymentDueAt = &due
	order.PaymentReminderSentAt = &sent

	row, err := BuildPaymentReminderOutboxRow(order)
	require.NoError(t, err)
	assert.Equal(t, "order-123", row.AggregateID, "key = order id")
	assert.Equal(t, "Order", row.AggregateType)
	assert.Equal(t, OrderPaymentReminderEventType, row.EventType)
	assert.Equal(t, "platform.order.v1.OrderPaymentReminder", OrderPaymentReminderEventType)

	env, ev := decodeReminder(t, row.Payload)
	assert.Equal(t, OrderPaymentReminderEventType, env.GetType())
	assert.Equal(t, row.EventID, env.GetEventId())
	assert.Equal(t, OrderPaymentReminderEventID("order-123"), env.GetEventId(), "event id is stable per order")
	assert.NotEqual(t, OrderPlacedEventID("order-123"), env.GetEventId(), "distinct from the other per-order facts")
	assert.Equal(t, "order-123", ev.GetOrderId())
	assert.Equal(t, "buyer-1", ev.GetBuyerId())
	assert.Equal(t, int64(250000), ev.GetTotalAmount())
	assert.Equal(t, "VND", ev.GetCurrency())
	require.NotNil(t, ev.GetPaymentDueAt())
	assert.True(t, due.Equal(ev.GetPaymentDueAt().AsTime()))
}

// Scenario: Order expiry: Repeated sweeps never repeat the reminder (the second
// guard: the event id is deterministic per order, so a re-enqueue is the same row).
func TestPaymentReminderEventIDIsDeterministicPerOrder(t *testing.T) {
	assert.Equal(t, OrderPaymentReminderEventID("a"), OrderPaymentReminderEventID("a"))
	assert.NotEqual(t, OrderPaymentReminderEventID("a"), OrderPaymentReminderEventID("b"))
}
