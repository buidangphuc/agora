package order_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	eventsv1 "github.com/buidangphuc/team-order/generated/platform/events/v1"
	orderv1 "github.com/buidangphuc/team-order/generated/platform/order/v1"
	"github.com/buidangphuc/team-order/internal/order"
	"github.com/buidangphuc/team-order/internal/store"
)

type receiptRig struct {
	svc     *order.OrderService
	orders  *store.PostgresOrderRepository
	returns *store.PostgresReturnRepository
	outbox  *pgOutbox
}

func newReceiptRig(t *testing.T) receiptRig {
	outbox := newPGOutbox(t)
	orders := outbox.orders(
		store.WithCompletedOutbox(BuildCompletedOutboxRow),
		store.WithReturnWindow(15*24*time.Hour))
	returns := outbox.returns()
	return receiptRig{svc: newSvc(t, orders, nil, returns, nil, nil, nil, nil), orders: orders, returns: returns, outbox: outbox}
}

// shipped creates a buyer_1 order and moves it to SHIPPED (paid online when paid).
func (r receiptRig) shipped(t *testing.T, paid bool) store.Order {
	t.Helper()
	ctx := context.Background()
	o, err := r.orders.CreateOrder(ctx, store.Order{BuyerID: "buyer_1", SellerID: "seller_1", TotalAmount: 5000, Currency: "VND"})
	if err != nil {
		t.Fatal(err)
	}
	from := store.OrderStatusPending
	if paid {
		if _, err := r.orders.UpdateOrderStatusFrom(ctx, o.ID, store.OrderStatusPaid, []store.OrderStatus{store.OrderStatusPending}, ""); err != nil {
			t.Fatal(err)
		}
		from = store.OrderStatusPaid
	}
	o, err = r.orders.UpdateOrderStatusFrom(ctx, o.ID, store.OrderStatusShipped, []store.OrderStatus{from}, "T")
	if err != nil {
		t.Fatal(err)
	}
	return o
}

func (r receiptRig) completedFacts(t *testing.T, orderID string) []*orderv1.OrderCompleted {
	t.Helper()
	var out []*orderv1.OrderCompleted
	for _, row := range r.outbox.EnqueuedRows() {
		if row.AggregateID != orderID || row.EventType != OrderCompletedEventType {
			continue
		}
		var env eventsv1.EventEnvelope
		if err := proto.Unmarshal(row.Payload, &env); err != nil {
			t.Fatal(err)
		}
		var ev orderv1.OrderCompleted
		if err := proto.Unmarshal(env.GetPayload(), &ev); err != nil {
			t.Fatal(err)
		}
		out = append(out, &ev)
	}
	return out
}

func (r receiptRig) status(t *testing.T, id string) store.OrderStatus {
	t.Helper()
	o, err := r.orders.GetOrder(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return o.Status
}

// Scenario: Order receipt: Buyer confirms a shipped order
func TestConfirmReceipt_BuyerConfirmsShippedOrder(t *testing.T) {
	r := newReceiptRig(t)
	o := r.shipped(t, true)
	got, err := r.svc.ConfirmReceipt(context.Background(), o.ID)
	if err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if got.Status != store.OrderStatusCompleted || got.CompletedAt == nil || got.ReturnWindowEndsAt == nil || got.AutoCompleteAt != nil {
		t.Fatalf("unexpected completed order: %+v", got)
	}
	if want := got.CompletedAt.Add(15 * 24 * time.Hour); !got.ReturnWindowEndsAt.Equal(want) {
		t.Fatalf("return window ends %v, want %v", got.ReturnWindowEndsAt, want)
	}
	if r.status(t, o.ID) != store.OrderStatusCompleted {
		t.Fatal("persisted status must be COMPLETED")
	}
	facts := r.completedFacts(t, o.ID)
	if len(facts) != 1 || facts[0].GetCompletedBy() != orderv1.CompletedBy_COMPLETED_BY_BUYER || !facts[0].GetPaidOnline() {
		t.Fatalf("want one BUYER OrderCompleted paid_online, got %v", facts)
	}
}

// Scenario: Order receipt: Buyer may confirm while their return is open
func TestConfirmReceiptAllowedWithOpenReturn(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name string
		to   store.ReturnStatus
	}{{"pending", store.ReturnStatusPending}, {"approved", store.ReturnStatusApproved}} {
		t.Run(tc.name, func(t *testing.T) {
			r := newReceiptRig(t)
			o := r.shipped(t, true)
			ret, err := r.returns.CreateReturn(ctx, store.OrderReturn{OrderID: o.ID, BuyerID: "buyer_1", SellerID: "seller_1", Reason: "x", RefundAmount: 1000, Status: store.ReturnStatusPending})
			if err != nil {
				t.Fatal(err)
			}
			if tc.to == store.ReturnStatusApproved {
				if _, err := r.returns.TransitionReturn(ctx, ret.ID, store.ReturnStatusPending, store.ReturnStatusApproved); err != nil {
					t.Fatal(err)
				}
			}
			got, err := r.svc.ConfirmReceipt(ctx, o.ID)
			if err != nil {
				t.Fatalf("confirm with open return: %v", err)
			}
			if got.Status != store.OrderStatusCompleted || got.ReturnWindowEndsAt == nil {
				t.Fatalf("unexpected order %+v", got)
			}
			after, err := r.returns.GetReturn(ctx, ret.ID)
			if err != nil || after.Status != tc.to {
				t.Fatalf("return must keep status %v, got %v (%v)", tc.to, after.Status, err)
			}
		})
	}
}

// Scenario: Order receipt: Confirming an order that is not shipped is refused
// and Scenario: Order receipt: A refused completion emits nothing
func TestConfirmReceipt_NotShippedIsRefusedAndEmitsNothing(t *testing.T) {
	ctx := context.Background()
	r := newReceiptRig(t)
	pending, err := r.orders.CreateOrder(ctx, store.Order{BuyerID: "buyer_1", SellerID: "seller_1", TotalAmount: 1})
	if err != nil {
		t.Fatal(err)
	}
	paid, err := r.orders.CreateOrder(ctx, store.Order{BuyerID: "buyer_1", SellerID: "seller_1", TotalAmount: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.orders.UpdateOrderStatusFrom(ctx, paid.ID, store.OrderStatusPaid, []store.OrderStatus{store.OrderStatusPending}, ""); err != nil {
		t.Fatal(err)
	}
	for name, o := range map[string]store.Order{"pending": pending, "paid": paid} {
		if _, err := r.svc.ConfirmReceipt(ctx, o.ID); !errors.Is(err, order.ErrInvalidStatus) {
			t.Fatalf("%s: want ErrInvalidStatus, got %v", name, err)
		}
		if n := len(r.completedFacts(t, o.ID)); n != 0 {
			t.Fatalf("%s: refused confirm emitted %d OrderCompleted", name, n)
		}
	}
	if r.status(t, pending.ID) != store.OrderStatusPending || r.status(t, paid.ID) != store.OrderStatusPaid {
		t.Fatal("refused confirm must leave statuses unchanged")
	}
}

// Scenario: Order receipt: Confirming an unknown order is not found
func TestConfirmReceipt_UnknownOrderNotFound(t *testing.T) {
	r := newReceiptRig(t)
	if _, err := r.svc.ConfirmReceipt(context.Background(), "ord_missing"); !errors.Is(err, store.ErrOrderNotFound) {
		t.Fatalf("want ErrOrderNotFound, got %v", err)
	}
}

// Scenario: Order receipt: Concurrent confirms emit one OrderCompleted
func TestConfirmReceipt_ConcurrentConfirmsEmitOneOrderCompleted(t *testing.T) {
	r := newReceiptRig(t)
	o := r.shipped(t, true)
	const n = 8
	errs := make([]error, n)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, errs[i] = r.svc.ConfirmReceipt(context.Background(), o.ID)
		}(i)
	}
	close(start)
	wg.Wait()
	wins := 0
	for _, err := range errs {
		switch {
		case err == nil:
			wins++
		case !errors.Is(err, order.ErrInvalidStatus):
			t.Fatalf("loser must get ErrInvalidStatus, got %v", err)
		}
	}
	if wins != 1 {
		t.Fatalf("want exactly one winner, got %d", wins)
	}
	if got := len(r.completedFacts(t, o.ID)); got != 1 {
		t.Fatalf("want one OrderCompleted, got %d", got)
	}
}

// Scenario: Order receipt: A cash-on-delivery completion is marked not paid online
// (service level: confirm of a COD order).
func TestConfirmReceipt_CODMarkedNotPaidOnline(t *testing.T) {
	r := newReceiptRig(t)
	o := r.shipped(t, false)
	if _, err := r.svc.ConfirmReceipt(context.Background(), o.ID); err != nil {
		t.Fatal(err)
	}
	facts := r.completedFacts(t, o.ID)
	if len(facts) != 1 || facts[0].GetPaidOnline() {
		t.Fatalf("want one OrderCompleted with paid_online=false, got %v", facts)
	}
}

// Scenario: Order receipt: Seller completion still works and emits OrderCompleted
func TestUpdateOrderStatus_SellerCompletionEmitsSellerOrderCompleted(t *testing.T) {
	r := newReceiptRig(t)
	o := r.shipped(t, true)
	if _, err := r.svc.UpdateOrderStatus(context.Background(), o.ID, order.ActorSeller, store.OrderStatusCompleted, ""); err != nil {
		t.Fatal(err)
	}
	facts := r.completedFacts(t, o.ID)
	if len(facts) != 1 || facts[0].GetCompletedBy() != orderv1.CompletedBy_COMPLETED_BY_SELLER {
		t.Fatalf("want one SELLER OrderCompleted, got %v", facts)
	}
}

// Scenario: Order receipt: A refused completion emits nothing
// (seller move of a PAID order to COMPLETED).
func TestUpdateOrderStatus_RefusedSellerCompletionEmitsNothing(t *testing.T) {
	ctx := context.Background()
	r := newReceiptRig(t)
	o, err := r.orders.CreateOrder(ctx, store.Order{BuyerID: "buyer_1", SellerID: "seller_1", TotalAmount: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.orders.UpdateOrderStatusFrom(ctx, o.ID, store.OrderStatusPaid, []store.OrderStatus{store.OrderStatusPending}, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.UpdateOrderStatus(ctx, o.ID, order.ActorSeller, store.OrderStatusCompleted, ""); !errors.Is(err, order.ErrInvalidStatus) {
		t.Fatalf("want ErrInvalidStatus, got %v", err)
	}
	if n := len(r.completedFacts(t, o.ID)); n != 0 {
		t.Fatalf("refused completion emitted %d OrderCompleted", n)
	}
}

// Scenario: Order receipt: A completed order cannot be reopened or cancelled
func TestConfirmReceipt_CompletedOrderCannotBeReopenedOrCancelled(t *testing.T) {
	ctx := context.Background()
	r := newReceiptRig(t)
	o := r.shipped(t, true)
	if _, err := r.svc.ConfirmReceipt(ctx, o.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.CancelOrder(ctx, o.ID); !errors.Is(err, order.ErrInvalidStatus) {
		t.Fatalf("cancel of completed order: want ErrInvalidStatus, got %v", err)
	}
	if _, err := r.svc.ConfirmReceipt(ctx, o.ID); !errors.Is(err, order.ErrInvalidStatus) {
		t.Fatalf("second confirm: want ErrInvalidStatus, got %v", err)
	}
	if r.status(t, o.ID) != store.OrderStatusCompleted {
		t.Fatal("order must stay COMPLETED")
	}
	if n := len(r.completedFacts(t, o.ID)); n != 1 {
		t.Fatalf("want exactly one OrderCompleted, got %d", n)
	}
}
