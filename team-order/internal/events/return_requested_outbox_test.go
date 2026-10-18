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
	"github.com/buidangphuc/team-order/internal/repository"
)

// Scenario: Order receipt: Return on a shipped order is accepted and announced
func TestBuildReturnRequestedOutboxRow(t *testing.T) {
	at := time.Date(2026, 10, 8, 9, 30, 0, 0, time.FixedZone("ICT", 7*3600))
	ret := repository.OrderReturn{ID: "ret-1", OrderID: "ord-1", BuyerID: "buyer-1", SellerID: "seller-1",
		Reason: "broken", RefundAmount: 200000, Status: repository.ReturnStatusPending, CreatedAt: at}

	row, err := BuildReturnRequestedOutboxRow(ret, "VND")
	require.NoError(t, err)
	assert.Equal(t, "return-requested:ret-1", ReturnRequestedEventID("ret-1"))
	assert.Equal(t, ReturnRequestedEventID("ret-1"), row.EventID)
	assert.Equal(t, "platform.order.v1.ReturnRequested", ReturnRequestedEventType)
	assert.Equal(t, ReturnRequestedEventType, row.EventType)
	assert.Equal(t, "Order", row.AggregateType)
	assert.Equal(t, "ord-1", row.AggregateID, "key = order id")

	var env eventsv1.EventEnvelope
	require.NoError(t, proto.Unmarshal(row.Payload, &env))
	assert.Equal(t, ReturnRequestedEventType, env.GetType())
	assert.Equal(t, row.EventID, env.GetEventId())
	var ev orderv1.ReturnRequested
	require.NoError(t, proto.Unmarshal(env.GetPayload(), &ev))
	assert.Equal(t, "ret-1", ev.GetReturnId())
	assert.Equal(t, "ord-1", ev.GetOrderId())
	assert.Equal(t, "buyer-1", ev.GetBuyerId())
	assert.Equal(t, "seller-1", ev.GetSellerId())
	assert.Equal(t, int64(200000), ev.GetRefundAmount())
	assert.Equal(t, "VND", ev.GetCurrency())
	assert.True(t, ev.GetRequestedAt().AsTime().Equal(at))

	other, err := BuildReturnRequestedOutboxRow(repository.OrderReturn{ID: "ret-2", OrderID: "ord-1"}, "VND")
	require.NoError(t, err)
	assert.NotEqual(t, row.EventID, other.EventID)
}

// Scenario: Order receipt: Return on a shipped order is accepted and announced
// The row is written with the insert; a refused (capped) request writes none.
func TestReturnRequestedOutbox_ViaRepository(t *testing.T) {
	ctx := context.Background()
	outbox := repository.NewInMemoryOutboxRepository()
	orders := repository.NewInMemoryOrderRepository()
	returns := repository.NewInMemoryReturnRepository(
		repository.WithReturnRequestedOutbox(BuildReturnRequestedOutboxRow),
		repository.WithInMemoryReturnOutbox(outbox, orders))
	o, err := orders.CreateOrder(ctx, repository.Order{BuyerID: "b", SellerID: "s", TotalAmount: 500000, Currency: "VND"})
	require.NoError(t, err)

	r, err := returns.CreateReturnCapped(ctx, repository.OrderReturn{OrderID: o.ID, BuyerID: "b", SellerID: "s", Reason: "x", RefundAmount: 200000}, o.TotalAmount)
	require.NoError(t, err)
	rows := outbox.EnqueuedRows()
	require.Len(t, rows, 1)
	assert.Equal(t, ReturnRequestedEventID(r.ID), rows[0].EventID)
	assert.Equal(t, o.ID, rows[0].AggregateID)

	_, err = returns.CreateReturnCapped(ctx, repository.OrderReturn{OrderID: o.ID, BuyerID: "b", SellerID: "s", Reason: "x", RefundAmount: 400000}, o.TotalAmount)
	require.ErrorIs(t, err, repository.ErrReturnExceedsRemainder)
	assert.Len(t, outbox.EnqueuedRows(), 1, "a refused request writes no row")
}

// Scenario: Order receipt: Return on a shipped order is accepted and announced
// If the row cannot be built the insert is rolled back (same transaction).
func TestReturnRequestedOutbox_BuildFailureRollsBackInsert(t *testing.T) {
	ctx := context.Background()
	outbox := repository.NewInMemoryOutboxRepository()
	orders := repository.NewInMemoryOrderRepository()
	boom := errors.New("boom")
	returns := repository.NewInMemoryReturnRepository(
		repository.WithReturnRequestedOutbox(func(repository.OrderReturn, string) (repository.OutboxRow, error) {
			return repository.OutboxRow{}, boom
		}),
		repository.WithInMemoryReturnOutbox(outbox, orders))
	o, err := orders.CreateOrder(ctx, repository.Order{BuyerID: "b", SellerID: "s", TotalAmount: 500000, Currency: "VND"})
	require.NoError(t, err)

	_, err = returns.CreateReturnCapped(ctx, repository.OrderReturn{OrderID: o.ID, BuyerID: "b", SellerID: "s", Reason: "x", RefundAmount: 200000}, o.TotalAmount)
	require.ErrorIs(t, err, boom)
	list, err := returns.ListReturnsByOrder(ctx, o.ID)
	require.NoError(t, err)
	assert.Empty(t, list, "the return must not persist without its outbox row")
	assert.Empty(t, outbox.EnqueuedRows())
}
