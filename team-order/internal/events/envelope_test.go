package events_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	eventsv1 "github.com/buidangphuc/team-order/generated/platform/events/v1"
	orderv1 "github.com/buidangphuc/team-order/generated/platform/order/v1"
	paymentv1 "github.com/buidangphuc/team-order/generated/platform/payment/v1"
	"github.com/buidangphuc/team-order/internal/events"
	"github.com/buidangphuc/team-order/internal/store"
)

func decodeEnvelope(t *testing.T, payload []byte, into proto.Message) *eventsv1.EventEnvelope {
	t.Helper()
	var env eventsv1.EventEnvelope
	require.NoError(t, proto.Unmarshal(payload, &env))
	require.NoError(t, proto.Unmarshal(env.GetPayload(), into))
	return &env
}

// Scenario: The event envelope contract is unchanged
func TestPaidEnvelopeMatchesContract(t *testing.T) {
	order := sampleOrder()
	order.Status = store.OrderStatusPaid
	order.UpdatedAt = time.Date(2026, 10, 5, 9, 30, 0, 0, time.UTC)

	row, err := events.BuildPaidOutboxRow(order)
	require.NoError(t, err)
	assert.Equal(t, "order-123", row.AggregateID)
	assert.Equal(t, "Order", row.AggregateType)
	assert.Equal(t, events.OrderPaidEventType, row.EventType)

	var ev orderv1.OrderPaidEvent
	env := decodeEnvelope(t, row.Payload, &ev)
	assert.Equal(t, "platform.order.v1.OrderPaidEvent", env.GetType())
	assert.Equal(t, row.EventID, env.GetEventId())
	assert.Equal(t, events.OrderPaidEventID("order-123"), env.GetEventId(), "event id is stable per order")
	assert.True(t, order.UpdatedAt.Equal(env.GetOccurredAt().AsTime()))

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

// Scenario: Order receipt: Payment stamps ship_by and the paid event carries it
func TestPaidEnvelopeCarriesSellerAndShipBy(t *testing.T) {
	paidAt := time.Date(2026, 10, 10, 8, 0, 0, 0, time.UTC)
	shipBy := paidAt.Add(48 * time.Hour)
	order := sampleOrder()
	order.Status = store.OrderStatusPaid
	order.UpdatedAt = paidAt
	order.PaidAt = &paidAt
	order.ShipBy = &shipBy

	row, err := events.BuildPaidOutboxRow(order)
	require.NoError(t, err)
	var ev orderv1.OrderPaidEvent
	decodeEnvelope(t, row.Payload, &ev)
	assert.Equal(t, "seller-1", ev.GetSellerId())
	require.NotNil(t, ev.GetShipBy())
	assert.True(t, shipBy.Equal(ev.GetShipBy().AsTime()))
}

// Scenario: The event envelope contract is unchanged
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

			row, err := events.BuildCancelledOutboxRow(order)
			require.NoError(t, err)
			assert.Equal(t, "order-123", row.AggregateID, "key = order id")
			assert.Equal(t, "Order", row.AggregateType)
			assert.Equal(t, events.OrderCancelledEventType, row.EventType)

			var ev orderv1.OrderCancelled
			env := decodeEnvelope(t, row.Payload, &ev)
			assert.Equal(t, "platform.order.v1.OrderCancelled", env.GetType())
			assert.Equal(t, row.EventID, env.GetEventId())
			assert.Equal(t, events.OrderCancelledEventID("order-123"), env.GetEventId(), "event id is stable per order")
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
	assert.NotEqual(t, events.OrderCancelledEventID("order-123"), events.OrderPaidEventID("order-123"),
		"the cancel fact and the paid fact of one order never share an id")
	assert.NotEqual(t, events.OrderCancelledEventID("order-123"), events.OrderCancelledEventID("order-456"))
}

// Scenario: Order expiry: Expiry emits OrderCancelled with the expiry reason
func TestCancelledEnvelopeCarriesReasonFromClaimedRow(t *testing.T) {
	for _, reason := range []orderv1.CancelReason{
		orderv1.CancelReason_CANCEL_REASON_UNSPECIFIED,
		orderv1.CancelReason_CANCEL_REASON_CHANGED_MIND,
		orderv1.CancelReason_CANCEL_REASON_PAYMENT_EXPIRED,
		orderv1.CancelReason_CANCEL_REASON_ADMIN_FORCE_FAIL,
	} {
		t.Run(reason.String(), func(t *testing.T) {
			order := sampleOrder()
			order.Status = store.OrderStatusCancelled
			order.CancelReason = int32(reason)
			row, err := events.BuildCancelledOutboxRow(order)
			require.NoError(t, err)
			var ev orderv1.OrderCancelled
			decodeEnvelope(t, row.Payload, &ev)
			assert.Equal(t, reason, ev.GetReasonCode())
		})
	}
}

func completedOrder(by orderv1.CompletedBy, paidAt *time.Time) store.Order {
	done := time.Date(2026, 10, 20, 8, 0, 0, 0, time.UTC)
	ends := done.Add(360 * time.Hour)
	o := sampleOrder()
	o.Status = store.OrderStatusCompleted
	o.PaidAt = paidAt
	o.CompletedAt = &done
	o.ReturnWindowEndsAt = &ends
	o.CompletedBy = int32(by)
	return o
}

// Scenario: Order receipt: Buyer confirm emits OrderCompleted with completed_by BUYER
func TestCompletedEnvelopeMatchesContract(t *testing.T) {
	paid := time.Date(2026, 10, 10, 8, 0, 0, 0, time.UTC)
	order := completedOrder(orderv1.CompletedBy_COMPLETED_BY_BUYER, &paid)

	row, err := events.BuildCompletedOutboxRow(order)
	require.NoError(t, err)
	assert.Equal(t, "order-123", row.AggregateID, "key = order id")
	assert.Equal(t, "Order", row.AggregateType)
	assert.Equal(t, events.OrderCompletedEventType, row.EventType)
	assert.Equal(t, "platform.order.v1.OrderCompleted", events.OrderCompletedEventType)
	assert.Equal(t, "order-completed:order-123", row.EventID)

	var ev orderv1.OrderCompleted
	env := decodeEnvelope(t, row.Payload, &ev)
	assert.Equal(t, events.OrderCompletedEventType, env.GetType())
	assert.Equal(t, row.EventID, env.GetEventId())
	assert.True(t, order.CompletedAt.Equal(env.GetOccurredAt().AsTime()))
	assert.Equal(t, "order-123", ev.GetOrderId())
	assert.Equal(t, "buyer-1", ev.GetBuyerId())
	assert.Equal(t, "seller-1", ev.GetSellerId())
	assert.Equal(t, int64(250000), ev.GetTotalAmount())
	assert.Equal(t, "VND", ev.GetCurrency())
	assert.Equal(t, orderv1.CompletedBy_COMPLETED_BY_BUYER, ev.GetCompletedBy())
	assert.True(t, ev.GetPaidOnline())
	assert.True(t, order.CompletedAt.Equal(ev.GetCompletedAt().AsTime()))
	assert.True(t, order.ReturnWindowEndsAt.Equal(ev.GetReturnWindowEndsAt().AsTime()))
}

// Scenario: Order receipt: Seller completion still works and emits OrderCompleted
func TestCompletedEnvelopeCarriesCompletedBy(t *testing.T) {
	for _, by := range []orderv1.CompletedBy{orderv1.CompletedBy_COMPLETED_BY_SELLER, orderv1.CompletedBy_COMPLETED_BY_SYSTEM} {
		row, err := events.BuildCompletedOutboxRow(completedOrder(by, nil))
		require.NoError(t, err)
		var ev orderv1.OrderCompleted
		decodeEnvelope(t, row.Payload, &ev)
		assert.Equal(t, by, ev.GetCompletedBy())
	}
}

// Scenario: Order receipt: A cash-on-delivery completion is marked not paid online
func TestCompletedPaidOnlineFalseWithoutPaidAt(t *testing.T) {
	order := completedOrder(orderv1.CompletedBy_COMPLETED_BY_BUYER, nil)
	order.PaymentMethod = int32(paymentv1.PaymentMethod_PAYMENT_METHOD_COD)
	row, err := events.BuildCompletedOutboxRow(order)
	require.NoError(t, err)
	var ev orderv1.OrderCompleted
	decodeEnvelope(t, row.Payload, &ev)
	assert.False(t, ev.GetPaidOnline())
}

// Scenario: Order expiry: Online-pay order gets a 24-hour deadline
func TestPlacedEnvelopeMatchesContract(t *testing.T) {
	placedAt := time.Date(2026, 10, 8, 9, 30, 0, 0, time.UTC)
	due := placedAt.Add(24 * time.Hour)
	order := sampleOrder()
	order.CreatedAt = placedAt
	order.UpdatedAt = placedAt
	order.PaymentMethod = int32(paymentv1.PaymentMethod_PAYMENT_METHOD_MOCK_MOMO)
	order.PaymentDueAt = &due

	row, err := events.BuildPlacedOutboxRow(order)
	require.NoError(t, err)
	assert.Equal(t, "order-123", row.AggregateID, "key = order id")
	assert.Equal(t, "Order", row.AggregateType)
	assert.Equal(t, events.OrderPlacedEventType, row.EventType)
	assert.Equal(t, "platform.order.v1.OrderPlaced", events.OrderPlacedEventType)

	var ev orderv1.OrderPlaced
	env := decodeEnvelope(t, row.Payload, &ev)
	assert.Equal(t, events.OrderPlacedEventType, env.GetType())
	assert.Equal(t, row.EventID, env.GetEventId())
	assert.Equal(t, events.OrderPlacedEventID("order-123"), env.GetEventId(), "event id is stable per order")
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

// Scenario: Order expiry: COD order has no deadline
func TestPlacedEnvelopeCodHasNoPaymentDueAt(t *testing.T) {
	order := sampleOrder()
	order.CreatedAt = time.Date(2026, 10, 8, 9, 30, 0, 0, time.UTC)
	order.PaymentMethod = int32(paymentv1.PaymentMethod_PAYMENT_METHOD_COD)
	row, err := events.BuildPlacedOutboxRow(order)
	require.NoError(t, err)
	var ev orderv1.OrderPlaced
	decodeEnvelope(t, row.Payload, &ev)
	assert.Nil(t, ev.GetPaymentDueAt())
	assert.Equal(t, paymentv1.PaymentMethod_PAYMENT_METHOD_COD, ev.GetPaymentMethod())
}

// Scenario: Order receipt: A cash-on-delivery order gets ship_by at placement
func TestPlacedEnvelopeCarriesShipByForCod(t *testing.T) {
	placedAt := time.Date(2026, 10, 8, 9, 30, 0, 0, time.UTC)
	shipBy := placedAt.Add(48 * time.Hour)
	order := sampleOrder()
	order.CreatedAt = placedAt
	order.PaymentMethod = int32(paymentv1.PaymentMethod_PAYMENT_METHOD_COD)
	order.ShipBy = &shipBy
	row, err := events.BuildPlacedOutboxRow(order)
	require.NoError(t, err)
	var ev orderv1.OrderPlaced
	decodeEnvelope(t, row.Payload, &ev)
	require.NotNil(t, ev.GetShipBy())
	assert.True(t, shipBy.Equal(ev.GetShipBy().AsTime()))
}

// Scenario: Order receipt: An unpaid online order has no ship_by
func TestPlacedEnvelopeHasNoShipByWhenOrderHasNone(t *testing.T) {
	order := sampleOrder()
	order.CreatedAt = time.Date(2026, 10, 8, 9, 30, 0, 0, time.UTC)
	order.PaymentMethod = int32(paymentv1.PaymentMethod_PAYMENT_METHOD_MOCK_MOMO)
	row, err := events.BuildPlacedOutboxRow(order)
	require.NoError(t, err)
	var ev orderv1.OrderPlaced
	decodeEnvelope(t, row.Payload, &ev)
	assert.Nil(t, ev.GetShipBy())
}

// Scenario: Order expiry: Each seller order of one checkout gets its own OrderPlaced
func TestPlacedEventIDIsPerOrderAndDistinctFromOtherFacts(t *testing.T) {
	assert.NotEqual(t, events.OrderPlacedEventID("a"), events.OrderPlacedEventID("b"))
	assert.Equal(t, events.OrderPlacedEventID("a"), events.OrderPlacedEventID("a"))
	assert.NotEqual(t, events.OrderPlacedEventID("a"), events.OrderCancelledEventID("a"))
	assert.NotEqual(t, events.OrderPlacedEventID("a"), events.OrderPaidEventID("a"))
}

// Scenario: Order expiry: Reminder is emitted two hours before the deadline
func TestPaymentReminderEnvelopeMatchesContract(t *testing.T) {
	due := time.Date(2026, 10, 9, 9, 30, 0, 0, time.UTC)
	sent := due.Add(-2 * time.Hour)
	order := sampleOrder()
	order.PaymentMethod = int32(paymentv1.PaymentMethod_PAYMENT_METHOD_MOCK_MOMO)
	order.PaymentDueAt = &due
	order.PaymentReminderSentAt = &sent

	row, err := events.BuildPaymentReminderOutboxRow(order)
	require.NoError(t, err)
	assert.Equal(t, "order-123", row.AggregateID, "key = order id")
	assert.Equal(t, "Order", row.AggregateType)
	assert.Equal(t, events.OrderPaymentReminderEventType, row.EventType)
	assert.Equal(t, "platform.order.v1.OrderPaymentReminder", events.OrderPaymentReminderEventType)

	var ev orderv1.OrderPaymentReminder
	env := decodeEnvelope(t, row.Payload, &ev)
	assert.Equal(t, events.OrderPaymentReminderEventType, env.GetType())
	assert.Equal(t, row.EventID, env.GetEventId())
	assert.Equal(t, events.OrderPaymentReminderEventID("order-123"), env.GetEventId(), "event id is stable per order")
	assert.NotEqual(t, events.OrderPlacedEventID("order-123"), env.GetEventId(), "distinct from the other per-order facts")
	assert.Equal(t, "order-123", ev.GetOrderId())
	assert.Equal(t, "buyer-1", ev.GetBuyerId())
	assert.Equal(t, int64(250000), ev.GetTotalAmount())
	assert.Equal(t, "VND", ev.GetCurrency())
	require.NotNil(t, ev.GetPaymentDueAt())
	assert.True(t, due.Equal(ev.GetPaymentDueAt().AsTime()))
}

// Scenario: Order expiry: Repeated sweeps never repeat the reminder
func TestPaymentReminderEventIDIsDeterministicPerOrder(t *testing.T) {
	assert.Equal(t, events.OrderPaymentReminderEventID("a"), events.OrderPaymentReminderEventID("a"))
	assert.NotEqual(t, events.OrderPaymentReminderEventID("a"), events.OrderPaymentReminderEventID("b"))
}

// Scenario: The event envelope contract is unchanged
func TestBuildReturnRefundedOutboxRow(t *testing.T) {
	at := time.Date(2026, 10, 8, 9, 30, 0, 0, time.FixedZone("ICT", 7*3600))
	ret := store.OrderReturn{ID: "ret-1", OrderID: "ord-1", BuyerID: "buyer-1", SellerID: "seller-1",
		Reason: "broken", RefundAmount: 200000, Status: store.ReturnStatusRefunded, UpdatedAt: at}

	row, err := events.BuildReturnRefundedOutboxRow(ret, "VND")
	require.NoError(t, err)
	assert.Equal(t, events.ReturnRefundedEventID("ret-1"), row.EventID)
	assert.Equal(t, "Order", row.AggregateType)
	assert.Equal(t, "ord-1", row.AggregateID, "key = order id")
	assert.Equal(t, events.ReturnRefundedEventType, row.EventType)

	var ev orderv1.ReturnRefunded
	env := decodeEnvelope(t, row.Payload, &ev)
	assert.Equal(t, "platform.order.v1.ReturnRefunded", env.GetType())
	assert.Equal(t, row.EventID, env.GetEventId())
	assert.True(t, env.GetOccurredAt().AsTime().Equal(at))
	assert.Equal(t, "ret-1", ev.GetReturnId())
	assert.Equal(t, "ord-1", ev.GetOrderId())
	assert.Equal(t, "buyer-1", ev.GetBuyerId())
	assert.Equal(t, "seller-1", ev.GetSellerId())
	assert.Equal(t, int64(200000), ev.GetRefundAmount())
	assert.Equal(t, "VND", ev.GetCurrency())
	assert.True(t, ev.GetRefundedAt().AsTime().Equal(at))

	again, err := events.BuildReturnRefundedOutboxRow(ret, "VND")
	require.NoError(t, err)
	assert.Equal(t, row.EventID, again.EventID)
	other, err := events.BuildReturnRefundedOutboxRow(store.OrderReturn{ID: "ret-2", OrderID: "ord-1"}, "VND")
	require.NoError(t, err)
	assert.NotEqual(t, row.EventID, other.EventID)
	assert.NotEqual(t, events.OrderCancelledEventID("ret-1"), row.EventID)
	assert.NotEqual(t, events.OrderShippedEventID("ret-1"), row.EventID)
}

// Scenario: Order receipt: Return on a shipped order is accepted and announced
func TestBuildReturnRequestedOutboxRow(t *testing.T) {
	at := time.Date(2026, 10, 8, 9, 30, 0, 0, time.FixedZone("ICT", 7*3600))
	ret := store.OrderReturn{ID: "ret-1", OrderID: "ord-1", BuyerID: "buyer-1", SellerID: "seller-1",
		Reason: "broken", RefundAmount: 200000, Status: store.ReturnStatusPending, CreatedAt: at}

	row, err := events.BuildReturnRequestedOutboxRow(ret, "VND")
	require.NoError(t, err)
	assert.Equal(t, "return-requested:ret-1", events.ReturnRequestedEventID("ret-1"))
	assert.Equal(t, events.ReturnRequestedEventID("ret-1"), row.EventID)
	assert.Equal(t, "platform.order.v1.ReturnRequested", events.ReturnRequestedEventType)
	assert.Equal(t, events.ReturnRequestedEventType, row.EventType)
	assert.Equal(t, "Order", row.AggregateType)
	assert.Equal(t, "ord-1", row.AggregateID, "key = order id")

	var ev orderv1.ReturnRequested
	env := decodeEnvelope(t, row.Payload, &ev)
	assert.Equal(t, events.ReturnRequestedEventType, env.GetType())
	assert.Equal(t, row.EventID, env.GetEventId())
	assert.Equal(t, "ret-1", ev.GetReturnId())
	assert.Equal(t, "ord-1", ev.GetOrderId())
	assert.Equal(t, "buyer-1", ev.GetBuyerId())
	assert.Equal(t, "seller-1", ev.GetSellerId())
	assert.Equal(t, int64(200000), ev.GetRefundAmount())
	assert.Equal(t, "VND", ev.GetCurrency())
	assert.True(t, ev.GetRequestedAt().AsTime().Equal(at))

	other, err := events.BuildReturnRequestedOutboxRow(store.OrderReturn{ID: "ret-2", OrderID: "ord-1"}, "VND")
	require.NoError(t, err)
	assert.NotEqual(t, row.EventID, other.EventID)
}

// Scenario: The event envelope contract is unchanged
func TestBuildShippedOutboxRow(t *testing.T) {
	at := time.Date(2026, 10, 8, 9, 30, 0, 0, time.UTC)
	row, err := events.BuildShippedOutboxRow(store.Shipment{
		ID: "shp-1", OrderID: "ord-1", Carrier: "GHN", TrackingCode: "GHN-123",
		BuyerID: "buyer-1", SellerID: "seller-1", CreatedAt: at,
	})
	require.NoError(t, err)
	assert.Equal(t, "ord-1", row.AggregateID)
	assert.Equal(t, "Order", row.AggregateType)
	assert.Equal(t, events.OrderShippedEventType, row.EventType)
	assert.Equal(t, events.OrderShippedEventID("shp-1"), row.EventID)

	var ev orderv1.OrderShipped
	env := decodeEnvelope(t, row.Payload, &ev)
	assert.Equal(t, row.EventID, env.GetEventId())
	assert.Equal(t, events.OrderShippedEventType, env.GetType())
	assert.Equal(t, "ord-1", ev.GetOrderId())
	assert.Equal(t, "buyer-1", ev.GetBuyerId())
	assert.Equal(t, "seller-1", ev.GetSellerId())
	assert.Equal(t, "GHN", ev.GetCarrier())
	assert.Equal(t, "GHN-123", ev.GetTrackingCode())
	assert.True(t, at.Equal(ev.GetShippedAt().AsTime()))
}
