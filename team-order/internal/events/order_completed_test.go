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
	"github.com/buidangphuc/team-order/internal/repository"
)

func completedOrder(by orderv1.CompletedBy, paidAt *time.Time) repository.Order {
	done := time.Date(2026, 10, 20, 8, 0, 0, 0, time.UTC)
	ends := done.Add(360 * time.Hour)
	o := sampleOrder()
	o.Status = repository.OrderStatusCompleted
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

	row, err := BuildCompletedOutboxRow(order)
	require.NoError(t, err)
	assert.Equal(t, "order-123", row.AggregateID, "key = order id")
	assert.Equal(t, "Order", row.AggregateType)
	assert.Equal(t, OrderCompletedEventType, row.EventType)
	assert.Equal(t, "platform.order.v1.OrderCompleted", OrderCompletedEventType)
	assert.Equal(t, "order-completed:order-123", row.EventID)

	var env eventsv1.EventEnvelope
	require.NoError(t, proto.Unmarshal(row.Payload, &env))
	assert.Equal(t, OrderCompletedEventType, env.GetType())
	assert.Equal(t, row.EventID, env.GetEventId())
	assert.True(t, order.CompletedAt.Equal(env.GetOccurredAt().AsTime()))
	var ev orderv1.OrderCompleted
	require.NoError(t, proto.Unmarshal(env.GetPayload(), &ev))
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
		row, err := BuildCompletedOutboxRow(completedOrder(by, nil))
		require.NoError(t, err)
		var env eventsv1.EventEnvelope
		require.NoError(t, proto.Unmarshal(row.Payload, &env))
		var ev orderv1.OrderCompleted
		require.NoError(t, proto.Unmarshal(env.GetPayload(), &ev))
		assert.Equal(t, by, ev.GetCompletedBy())
	}
}

// Scenario: Order receipt: A cash-on-delivery completion is marked not paid online
func TestCompletedPaidOnlineFalseWithoutPaidAt(t *testing.T) {
	order := completedOrder(orderv1.CompletedBy_COMPLETED_BY_BUYER, nil)
	order.PaymentMethod = int32(paymentv1.PaymentMethod_PAYMENT_METHOD_COD)
	row, err := BuildCompletedOutboxRow(order)
	require.NoError(t, err)
	var env eventsv1.EventEnvelope
	require.NoError(t, proto.Unmarshal(row.Payload, &env))
	var ev orderv1.OrderCompleted
	require.NoError(t, proto.Unmarshal(env.GetPayload(), &ev))
	assert.False(t, ev.GetPaidOnline())
}

// Scenario: Order receipt: Payment stamps ship_by and the paid event carries it
func TestPaidEnvelopeCarriesSellerAndShipBy(t *testing.T) {
	paidAt := time.Date(2026, 10, 10, 8, 0, 0, 0, time.UTC)
	shipBy := paidAt.Add(48 * time.Hour)
	order := sampleOrder()
	order.Status = repository.OrderStatusPaid
	order.UpdatedAt = paidAt
	order.PaidAt = &paidAt
	order.ShipBy = &shipBy

	row, err := BuildPaidOutboxRow(order)
	require.NoError(t, err)
	var env eventsv1.EventEnvelope
	require.NoError(t, proto.Unmarshal(row.Payload, &env))
	var ev orderv1.OrderPaidEvent
	require.NoError(t, proto.Unmarshal(env.GetPayload(), &ev))
	assert.Equal(t, "seller-1", ev.GetSellerId())
	require.NotNil(t, ev.GetShipBy())
	assert.True(t, shipBy.Equal(ev.GetShipBy().AsTime()))
}

// Scenario: Order receipt: A cash-on-delivery order gets ship_by at placement
func TestPlacedEnvelopeCarriesShipByForCod(t *testing.T) {
	placedAt := time.Date(2026, 10, 8, 9, 30, 0, 0, time.UTC)
	shipBy := placedAt.Add(48 * time.Hour)
	order := sampleOrder()
	order.CreatedAt = placedAt
	order.PaymentMethod = int32(paymentv1.PaymentMethod_PAYMENT_METHOD_COD)
	order.ShipBy = &shipBy
	row, err := BuildPlacedOutboxRow(order)
	require.NoError(t, err)
	_, ev := decodePlaced(t, row.Payload)
	require.NotNil(t, ev.GetShipBy())
	assert.True(t, shipBy.Equal(ev.GetShipBy().AsTime()))
}

// Scenario: Order receipt: An unpaid online order has no ship_by
func TestPlacedEnvelopeHasNoShipByWhenOrderHasNone(t *testing.T) {
	order := sampleOrder()
	order.CreatedAt = time.Date(2026, 10, 8, 9, 30, 0, 0, time.UTC)
	order.PaymentMethod = int32(paymentv1.PaymentMethod_PAYMENT_METHOD_MOCK_MOMO)
	row, err := BuildPlacedOutboxRow(order)
	require.NoError(t, err)
	_, ev := decodePlaced(t, row.Payload)
	assert.Nil(t, ev.GetShipBy())
}
