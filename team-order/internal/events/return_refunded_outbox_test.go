package events

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	eventsv1 "github.com/buidangphuc/team-order/generated/platform/events/v1"
	orderv1 "github.com/buidangphuc/team-order/generated/platform/order/v1"
	"github.com/buidangphuc/team-order/internal/repository"
)

func decodeReturnRefunded(t *testing.T, payload []byte) (*eventsv1.EventEnvelope, *orderv1.ReturnRefunded) {
	t.Helper()
	var env eventsv1.EventEnvelope
	require.NoError(t, proto.Unmarshal(payload, &env))
	var ev orderv1.ReturnRefunded
	require.NoError(t, proto.Unmarshal(env.GetPayload(), &ev))
	return &env, &ev
}

// The row matches the order.proto contract: an EventEnvelope of type
// platform.order.v1.ReturnRefunded, keyed by order id, every field from the
// stored return and the order currency, and an event id stable per return.
func TestBuildReturnRefundedOutboxRow(t *testing.T) {
	at := time.Date(2026, 10, 8, 9, 30, 0, 0, time.FixedZone("ICT", 7*3600))
	ret := repository.OrderReturn{ID: "ret-1", OrderID: "ord-1", BuyerID: "buyer-1", SellerID: "seller-1",
		Reason: "broken", RefundAmount: 200000, Status: repository.ReturnStatusRefunded, UpdatedAt: at}

	row, err := BuildReturnRefundedOutboxRow(ret, "VND")
	require.NoError(t, err)
	assert.Equal(t, ReturnRefundedEventID("ret-1"), row.EventID)
	assert.Equal(t, "Order", row.AggregateType)
	assert.Equal(t, "ord-1", row.AggregateID, "key = order id")
	assert.Equal(t, ReturnRefundedEventType, row.EventType)

	env, ev := decodeReturnRefunded(t, row.Payload)
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

	// Stable across builds; distinct per return; never another fact's id.
	again, err := BuildReturnRefundedOutboxRow(ret, "VND")
	require.NoError(t, err)
	assert.Equal(t, row.EventID, again.EventID)
	other, err := BuildReturnRefundedOutboxRow(repository.OrderReturn{ID: "ret-2", OrderID: "ord-1"}, "VND")
	require.NoError(t, err)
	assert.NotEqual(t, row.EventID, other.EventID)
	assert.NotEqual(t, OrderCancelledEventID("ret-1"), row.EventID)
	assert.NotEqual(t, OrderShippedEventID("ret-1"), row.EventID)
}

// Wired into the in-memory return repository, a won APPROVED -> REFUNDED writes
// one ReturnRefunded row with the stored amount and the order currency.
func TestReturnRefundedOutbox_ViaRepository(t *testing.T) {
	ctx := context.Background()
	outbox := repository.NewInMemoryOutboxRepository()
	orders := repository.NewInMemoryOrderRepository()
	returns := repository.NewInMemoryReturnRepository(repository.WithReturnOutbox(BuildReturnRefundedOutboxRow),
		repository.WithInMemoryReturnOutbox(outbox, orders))
	o, err := orders.CreateOrder(ctx, repository.Order{BuyerID: "b", SellerID: "s", TotalAmount: 500000, Currency: "VND"})
	require.NoError(t, err)
	r, err := returns.CreateReturnCapped(ctx, repository.OrderReturn{OrderID: o.ID, BuyerID: "b", SellerID: "s",
		Reason: "x", RefundAmount: 200000}, o.TotalAmount)
	require.NoError(t, err)
	_, err = returns.TransitionReturn(ctx, r.ID, repository.ReturnStatusPending, repository.ReturnStatusApproved)
	require.NoError(t, err)
	_, err = returns.TransitionReturn(ctx, r.ID, repository.ReturnStatusApproved, repository.ReturnStatusRefunded)
	require.NoError(t, err)

	rows := outbox.EnqueuedRows()
	require.Len(t, rows, 1)
	_, ev := decodeReturnRefunded(t, rows[0].Payload)
	assert.Equal(t, r.ID, ev.GetReturnId())
	assert.Equal(t, int64(200000), ev.GetRefundAmount())
	assert.Equal(t, "VND", ev.GetCurrency())
	assert.Equal(t, o.ID, rows[0].AggregateID)
}
