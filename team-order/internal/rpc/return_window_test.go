package rpc_test

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	orderv1 "github.com/buidangphuc/team-order/generated/platform/order/v1"
	"github.com/buidangphuc/team-order/internal/order"
	"github.com/buidangphuc/team-order/internal/rpc"
	"github.com/buidangphuc/team-order/internal/store"
)

type windowEnv struct {
	h      *rpc.OrderHandler
	orders *store.PostgresOrderRepository
	outbox *pgOutbox
	svc    *order.OrderService
	now    time.Time
}

func newWindowEnv(t testing.TB) *windowEnv {
	e := &windowEnv{now: time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)}
	e.outbox = newPGOutbox(t.Context(), t)
	e.orders = e.outbox.orders(store.WithCompletedOutbox(BuildCompletedOutboxRow),
		store.WithReturnWindow(15*24*time.Hour))
	returns := e.outbox.returns(store.WithReturnRequestedOutbox(BuildReturnRequestedOutboxRow))
	e.svc = newSvc(t, e.orders, nil, returns, nil, nil, nil, nil, order.WithClock(func() time.Time { return e.now }))
	e.h = newHandler(t, e.svc, nil, nil, withAudit(&fakeAudit{}))
	return e
}

func (e *windowEnv) order(t *testing.T, to store.OrderStatus) store.Order {
	t.Helper()
	ctx := context.Background()
	o, err := e.orders.CreateOrder(ctx, store.Order{BuyerID: "buyer_1", SellerID: "seller_1", TotalAmount: 500000, Currency: "VND"})
	if err != nil {
		t.Fatal(err)
	}
	chain := map[store.OrderStatus][]store.OrderStatus{
		store.OrderStatusPaid:      {store.OrderStatusPaid},
		store.OrderStatusShipped:   {store.OrderStatusPaid, store.OrderStatusShipped},
		store.OrderStatusCompleted: {store.OrderStatusPaid, store.OrderStatusShipped},
	}[to]
	prev := store.OrderStatusPending
	for _, st := range chain {
		if o, err = e.orders.UpdateOrderStatusFrom(ctx, o.ID, st, []store.OrderStatus{prev}, "T"); err != nil {
			t.Fatal(err)
		}
		prev = st
	}
	if to == store.OrderStatusCompleted {
		if o, err = e.svc.ConfirmReceipt(ctx, o.ID); err != nil {
			t.Fatal(err)
		}
	}
	return o
}

func (e *windowEnv) create(orderID string) error {
	_, err := e.h.CreateReturnRequest(incomingPrincipalCtx("buyer_1", "buyer"),
		&orderv1.CreateReturnRequestRequest{OrderId: orderID, Reason: "broken", RefundAmount: 100000})
	return err
}

// Scenario: Order receipt: Return after the window closes is refused
func TestCreateReturnRequestWindowClosedIsFailedPrecondition(t *testing.T) {
	e := newWindowEnv(t)
	o := e.order(t, store.OrderStatusCompleted)
	e.now = o.ReturnWindowEndsAt.Add(time.Second)
	err := e.create(o.ID)
	if status.Code(err) != codes.FailedPrecondition || status.Convert(err).Message() != "return window closed" {
		t.Fatalf("want FAILED_PRECONDITION 'return window closed', got %v", err)
	}
}

// Scenario: Order receipt: Return at the exact window end is refused
func TestCreateReturnRequestAtWindowEndIsFailedPrecondition(t *testing.T) {
	e := newWindowEnv(t)
	o := e.order(t, store.OrderStatusCompleted)
	e.now = *o.ReturnWindowEndsAt
	err := e.create(o.ID)
	if status.Code(err) != codes.FailedPrecondition || status.Convert(err).Message() != "return window closed" {
		t.Fatalf("want FAILED_PRECONDITION 'return window closed', got %v", err)
	}
}

// Scenario: Order receipt: Return on a paid unshipped order is refused
func TestCreateReturnRequestPaidOrderIsFailedPrecondition(t *testing.T) {
	e := newWindowEnv(t)
	o := e.order(t, store.OrderStatusPaid)
	err := e.create(o.ID)
	if status.Code(err) != codes.FailedPrecondition || status.Convert(err).Message() != "order cannot be returned in its current status" {
		t.Fatalf("want FAILED_PRECONDITION status message, got %v", err)
	}
}

// Scenario: Order receipt: Return inside the window after completion is accepted
func TestCreateReturnRequestInsideWindowAccepted(t *testing.T) {
	e := newWindowEnv(t)
	o := e.order(t, store.OrderStatusCompleted)
	e.now = o.ReturnWindowEndsAt.Add(-time.Second)
	if err := e.create(o.ID); err != nil {
		t.Fatalf("want accepted, got %v", err)
	}
}

// Scenario: Order receipt: Another buyer cannot return the order
func TestCreateReturnRequestOtherBuyerIsPermissionDenied(t *testing.T) {
	e := newWindowEnv(t)
	o := e.order(t, store.OrderStatusShipped)
	_, err := e.h.CreateReturnRequest(incomingPrincipalCtx("buyer_2", "buyer"),
		&orderv1.CreateReturnRequestRequest{OrderId: o.ID, Reason: "broken", RefundAmount: 100000})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("want PERMISSION_DENIED, got %v", err)
	}
}

// Scenario: Order receipt: Return above the cap is still refused
func TestCreateReturnRequestAboveCapIsInvalidArgument(t *testing.T) {
	e := newWindowEnv(t)
	o := e.order(t, store.OrderStatusShipped)
	_, err := e.h.CreateReturnRequest(incomingPrincipalCtx("buyer_1", "buyer"),
		&orderv1.CreateReturnRequestRequest{OrderId: o.ID, Reason: "broken", RefundAmount: 900000})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("want INVALID_ARGUMENT, got %v", err)
	}
}
