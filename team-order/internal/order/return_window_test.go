package order_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	eventsv1 "github.com/buidangphuc/team-order/generated/platform/events/v1"
	orderv1 "github.com/buidangphuc/team-order/generated/platform/order/v1"
	"github.com/buidangphuc/team-order/internal/order"
	"github.com/buidangphuc/team-order/internal/store"
)

// returnRig wires the order, return and outbox stores the way main.go does, with a
// controllable service clock (order.WithClock) so window edges are exact.
type returnRig struct {
	svc     *order.OrderService
	orders  *store.PostgresOrderRepository
	returns *store.PostgresReturnRepository
	outbox  *pgOutbox
	now     time.Time
}

func newReturnRig(t *testing.T) *returnRig {
	t.Helper()
	r := &returnRig{now: time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)}
	r.outbox = newPGOutbox(t.Context(), t)
	r.orders = r.outbox.orders(
		store.WithCompletedOutbox(BuildCompletedOutboxRow),
		store.WithReturnWindow(15*24*time.Hour))
	r.returns = r.outbox.returns(
		store.WithReturnOutbox(BuildReturnRefundedOutboxRow),
		store.WithReturnRequestedOutbox(BuildReturnRequestedOutboxRow))
	r.svc = newSvc(t, r.orders, nil, r.returns, nil, nil, nil, nil,
		order.WithClock(func() time.Time { return r.now }))
	return r
}

func (r *returnRig) order(t *testing.T, to store.OrderStatus) store.Order {
	t.Helper()
	ctx := context.Background()
	o, err := r.orders.CreateOrder(ctx, store.Order{BuyerID: "buyer_1", SellerID: "seller_1", TotalAmount: 500000, Currency: "VND"})
	if err != nil {
		t.Fatal(err)
	}
	step := func(to store.OrderStatus, from store.OrderStatus) {
		if o, err = r.orders.UpdateOrderStatusFrom(ctx, o.ID, to, []store.OrderStatus{from}, "T"); err != nil {
			t.Fatal(err)
		}
	}
	switch to {
	case store.OrderStatusPending:
	case store.OrderStatusCancelled:
		step(to, store.OrderStatusPending)
	case store.OrderStatusPaid:
		step(to, store.OrderStatusPending)
	case store.OrderStatusShipped:
		step(store.OrderStatusPaid, store.OrderStatusPending)
		step(to, store.OrderStatusPaid)
	case store.OrderStatusCompleted:
		step(store.OrderStatusPaid, store.OrderStatusPending)
		step(store.OrderStatusShipped, store.OrderStatusPaid)
		if o, err = r.svc.ConfirmReceipt(ctx, o.ID); err != nil {
			t.Fatal(err)
		}
	}
	return o
}

func (r *returnRig) requested(t *testing.T) []*orderv1.ReturnRequested {
	t.Helper()
	var out []*orderv1.ReturnRequested
	for _, row := range r.outbox.EnqueuedRows(t.Context()) {
		if row.EventType != ReturnRequestedEventType {
			continue
		}
		var env eventsv1.EventEnvelope
		if err := proto.Unmarshal(row.Payload, &env); err != nil {
			t.Fatal(err)
		}
		var ev orderv1.ReturnRequested
		if err := proto.Unmarshal(env.GetPayload(), &ev); err != nil {
			t.Fatal(err)
		}
		out = append(out, &ev)
	}
	return out
}

func (r *returnRig) returnsOf(t *testing.T, orderID string) []store.OrderReturn {
	t.Helper()
	list, err := r.returns.ListReturnsByOrder(context.Background(), orderID)
	if err != nil {
		t.Fatal(err)
	}
	return list
}

// Scenario: Order receipt: Return on a shipped order is accepted and announced
func TestCreateReturnOnShippedOrderAcceptedAndAnnounced(t *testing.T) {
	r := newReturnRig(t)
	o := r.order(t, store.OrderStatusShipped)
	ret, err := r.svc.CreateReturnRequest(context.Background(), "buyer_1", o.ID, "broken", 200000)
	if err != nil {
		t.Fatalf("create return: %v", err)
	}
	if ret.Status != store.ReturnStatusPending {
		t.Fatalf("want PENDING, got %v", ret.Status)
	}
	got := r.requested(t)
	if len(got) != 1 {
		t.Fatalf("want exactly one ReturnRequested, got %d", len(got))
	}
	ev := got[0]
	if ev.GetReturnId() != ret.ID || ev.GetOrderId() != o.ID || ev.GetBuyerId() != "buyer_1" ||
		ev.GetSellerId() != "seller_1" || ev.GetRefundAmount() != 200000 || ev.GetCurrency() != "VND" || ev.GetRequestedAt() == nil {
		t.Fatalf("unexpected ReturnRequested: %v", ev)
	}
	for _, row := range r.outbox.EnqueuedRows(t.Context()) {
		if row.EventType == ReturnRequestedEventType && (row.EventID != ReturnRequestedEventID(ret.ID) || row.AggregateID != o.ID) {
			t.Fatalf("row keyed %q id %q, want order id and return-requested id", row.AggregateID, row.EventID)
		}
	}
}

// Scenario: Order receipt: Return inside the window after completion is accepted
func TestCreateReturnInsideWindowAfterCompletionAccepted(t *testing.T) {
	r := newReturnRig(t)
	o := r.order(t, store.OrderStatusCompleted)
	r.now = o.ReturnWindowEndsAt.Add(-time.Second)
	ret, err := r.svc.CreateReturnRequest(context.Background(), "buyer_1", o.ID, "broken", 100000)
	if err != nil {
		t.Fatalf("create return: %v", err)
	}
	if ret.Status != store.ReturnStatusPending {
		t.Fatalf("want PENDING, got %v", ret.Status)
	}
	if n := len(r.requested(t)); n != 1 {
		t.Fatalf("want one ReturnRequested, got %d", n)
	}
}

// Scenario: Order receipt: Return after the window closes is refused
func TestCreateReturnAfterWindowClosedRefused(t *testing.T) {
	r := newReturnRig(t)
	o := r.order(t, store.OrderStatusCompleted)
	r.now = o.ReturnWindowEndsAt.Add(time.Second)
	_, err := r.svc.CreateReturnRequest(context.Background(), "buyer_1", o.ID, "broken", 100000)
	if !errors.Is(err, order.ErrReturnWindowClosed) {
		t.Fatalf("want ErrReturnWindowClosed, got %v", err)
	}
	if len(r.returnsOf(t, o.ID)) != 0 || len(r.requested(t)) != 0 {
		t.Fatal("a refused return must leave no return and no ReturnRequested")
	}
}

// Scenario: Order receipt: Return at the exact window end is refused
func TestCreateReturnAtWindowEndRefused(t *testing.T) {
	r := newReturnRig(t)
	o := r.order(t, store.OrderStatusCompleted)
	r.now = *o.ReturnWindowEndsAt
	_, err := r.svc.CreateReturnRequest(context.Background(), "buyer_1", o.ID, "broken", 100000)
	if !errors.Is(err, order.ErrReturnWindowClosed) {
		t.Fatalf("want ErrReturnWindowClosed at the exact end, got %v", err)
	}
	if len(r.requested(t)) != 0 {
		t.Fatal("no ReturnRequested expected")
	}
}

// Scenario: Order receipt: Return on a paid unshipped order is refused
func TestCreateReturnOnPaidUnshippedRefused(t *testing.T) {
	r := newReturnRig(t)
	o := r.order(t, store.OrderStatusPaid)
	_, err := r.svc.CreateReturnRequest(context.Background(), "buyer_1", o.ID, "broken", 100000)
	if !errors.Is(err, order.ErrOrderCannotBeReturned) {
		t.Fatalf("want ErrOrderCannotBeReturned, got %v", err)
	}
	if errors.Is(err, order.ErrReturnWindowClosed) {
		t.Fatal("a PAID order is a status refusal, not a window refusal")
	}
	if len(r.returnsOf(t, o.ID)) != 0 || len(r.requested(t)) != 0 {
		t.Fatal("a refused return must leave no return and no ReturnRequested")
	}
}

// Scenario: Order receipt: Return on a pending or cancelled order is refused
func TestCreateReturnPendingOrCancelledRefused(t *testing.T) {
	for _, st := range []store.OrderStatus{store.OrderStatusPending, store.OrderStatusCancelled} {
		r := newReturnRig(t)
		o := r.order(t, st)
		_, err := r.svc.CreateReturnRequest(context.Background(), "buyer_1", o.ID, "broken", 100000)
		if !errors.Is(err, order.ErrOrderCannotBeReturned) {
			t.Fatalf("status %v: want ErrOrderCannotBeReturned, got %v", st, err)
		}
		if len(r.returnsOf(t, o.ID)) != 0 || len(r.requested(t)) != 0 {
			t.Fatalf("status %v: refused return left state", st)
		}
	}
}

// Scenario: Order receipt: Another buyer cannot return the order
func TestCreateReturnOtherBuyerDenied(t *testing.T) {
	r := newReturnRig(t)
	o := r.order(t, store.OrderStatusShipped)
	_, err := r.svc.CreateReturnRequest(context.Background(), "buyer_2", o.ID, "broken", 100000)
	if !errors.Is(err, order.ErrUnauthorizedReturn) {
		t.Fatalf("want ErrUnauthorizedReturn, got %v", err)
	}
	if len(r.returnsOf(t, o.ID)) != 0 || len(r.requested(t)) != 0 {
		t.Fatal("a denied return must leave no return and no ReturnRequested")
	}
}

// Scenario: Order receipt: Return above the cap is still refused
func TestCreateReturnCapStillEnforced(t *testing.T) {
	r := newReturnRig(t)
	o := r.order(t, store.OrderStatusShipped)
	ctx := context.Background()
	if _, err := r.svc.CreateReturnRequest(ctx, "buyer_1", o.ID, "first", 400000); err != nil {
		t.Fatalf("first return: %v", err)
	}
	_, err := r.svc.CreateReturnRequest(ctx, "buyer_1", o.ID, "second", 200000)
	if !errors.Is(err, order.ErrInvalidRefundAmount) {
		t.Fatalf("want ErrInvalidRefundAmount, got %v", err)
	}
	if n := len(r.requested(t)); n != 1 {
		t.Fatalf("only the accepted return is announced, got %d", n)
	}
}
