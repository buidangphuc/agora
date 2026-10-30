package rpc_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	eventsv1 "github.com/buidangphuc/team-order/generated/platform/events/v1"
	orderv1 "github.com/buidangphuc/team-order/generated/platform/order/v1"
	"github.com/buidangphuc/team-order/internal/rpc"
	"github.com/buidangphuc/team-order/internal/store"
)

type receiptEnv struct {
	h      *rpc.OrderHandler
	orders *store.PostgresOrderRepository
	outbox *pgOutbox
}

func newReceiptEnv(t testing.TB) receiptEnv {
	outbox := newPGOutbox(t.Context(), t)
	orders := outbox.orders(store.WithCompletedOutbox(BuildCompletedOutboxRow), store.WithReturnWindow(15*24*time.Hour))
	returns := outbox.returns()
	svc := newSvc(t, orders, nil, returns, nil, nil, nil, nil)
	return receiptEnv{h: newHandler(t, svc, nil, nil, withAudit(&fakeAudit{})), orders: orders, outbox: outbox}
}

func (e receiptEnv) order(t *testing.T, to store.OrderStatus) store.Order {
	t.Helper()
	ctx := context.Background()
	o, err := e.orders.CreateOrder(ctx, store.Order{BuyerID: "buyer_1", SellerID: "seller_1", TotalAmount: 5000, Currency: "VND"})
	if err != nil {
		t.Fatal(err)
	}
	if to == store.OrderStatusPending {
		return o
	}
	if o, err = e.orders.UpdateOrderStatusFrom(ctx, o.ID, store.OrderStatusPaid, []store.OrderStatus{store.OrderStatusPending}, ""); err != nil {
		t.Fatal(err)
	}
	if to == store.OrderStatusPaid {
		return o
	}
	if o, err = e.orders.UpdateOrderStatusFrom(ctx, o.ID, store.OrderStatusShipped, []store.OrderStatus{store.OrderStatusPaid}, "T"); err != nil {
		t.Fatal(err)
	}
	return o
}

func (e receiptEnv) completedCount(t *testing.T, orderID string) (n int, by []orderv1.CompletedBy) {
	t.Helper()
	for _, row := range e.outbox.EnqueuedRows(t.Context()) {
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
		n++
		by = append(by, ev.GetCompletedBy())
	}
	return n, by
}

func (e receiptEnv) statusOf(t *testing.T, id string) store.OrderStatus {
	t.Helper()
	o, err := e.orders.GetOrder(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return o.Status
}

func wantReceiptCode(t *testing.T, err error, want codes.Code) {
	t.Helper()
	if status.Code(err) != want {
		t.Fatalf("want %v, got %v (%v)", want, status.Code(err), err)
	}
}

// Scenario: Order receipt: Buyer confirms a shipped order
func TestConfirmReceiptBuyerConfirmsShippedOrder(t *testing.T) {
	e := newReceiptEnv(t)
	o := e.order(t, store.OrderStatusShipped)
	res, err := e.h.ConfirmReceipt(incomingPrincipalCtx("buyer_1", "buyer"), &orderv1.ConfirmReceiptRequest{Id: o.ID})
	if err != nil {
		t.Fatalf("confirm: %v", err)
	}
	w := res.GetOrder()
	if w.GetId() != o.ID || w.GetStatus() != orderv1.OrderStatus_ORDER_STATUS_COMPLETED {
		t.Fatalf("want COMPLETED %s, got %v", o.ID, w)
	}
	if w.GetCompletedAt() == nil || w.GetReturnWindowEndsAt() == nil || w.GetAutoCompleteAt() != nil {
		t.Fatalf("completion stamps wrong: %v", w)
	}
	if n, by := e.completedCount(t, o.ID); n != 1 || by[0] != orderv1.CompletedBy_COMPLETED_BY_BUYER {
		t.Fatalf("want one BUYER OrderCompleted, got %d %v", n, by)
	}
}

// Scenario: Order receipt: Another user cannot confirm receipt
// and Scenario: Order receipt: A refused completion emits nothing
func TestConfirmReceiptNonBuyerIsPermissionDenied(t *testing.T) {
	for _, tc := range []struct {
		name string
		to   store.OrderStatus
		ctx  context.Context
	}{
		{"seller", store.OrderStatusShipped, incomingPrincipalCtx("seller_1", "seller")},
		{"other buyer", store.OrderStatusShipped, incomingPrincipalCtx("buyer_2", "buyer")},
		{"service", store.OrderStatusShipped, serviceCtx("order.write")},
		// permission is decided before the status is revealed
		{"other buyer on a paid order", store.OrderStatusPaid, incomingPrincipalCtx("buyer_2", "buyer")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newReceiptEnv(t)
			o := e.order(t, tc.to)
			_, err := e.h.ConfirmReceipt(tc.ctx, &orderv1.ConfirmReceiptRequest{Id: o.ID})
			wantReceiptCode(t, err, codes.PermissionDenied)
			if e.statusOf(t, o.ID) != tc.to {
				t.Fatal("order must be unchanged")
			}
			if n, _ := e.completedCount(t, o.ID); n != 0 {
				t.Fatalf("refused confirm emitted %d OrderCompleted", n)
			}
		})
	}
}

// Scenario: Order receipt: Confirming an order that is not shipped is refused
// and Scenario: Order receipt: A refused completion emits nothing
func TestConfirmReceiptNotShippedIsFailedPrecondition(t *testing.T) {
	for name, to := range map[string]store.OrderStatus{"pending": store.OrderStatusPending, "paid": store.OrderStatusPaid} {
		t.Run(name, func(t *testing.T) {
			e := newReceiptEnv(t)
			o := e.order(t, to)
			_, err := e.h.ConfirmReceipt(incomingPrincipalCtx("buyer_1", "buyer"), &orderv1.ConfirmReceiptRequest{Id: o.ID})
			wantReceiptCode(t, err, codes.FailedPrecondition)
			if e.statusOf(t, o.ID) != to {
				t.Fatal("order must be unchanged")
			}
			if n, _ := e.completedCount(t, o.ID); n != 0 {
				t.Fatalf("refused confirm emitted %d OrderCompleted", n)
			}
		})
	}
}

// Scenario: Order receipt: Confirming an unknown order is not found
func TestConfirmReceiptUnknownOrderNotFound(t *testing.T) {
	e := newReceiptEnv(t)
	_, err := e.h.ConfirmReceipt(incomingPrincipalCtx("buyer_1", "buyer"), &orderv1.ConfirmReceiptRequest{Id: "ord_missing"})
	wantReceiptCode(t, err, codes.NotFound)
}

// Scenario: Order receipt: Confirming an order that is not shipped is refused
// (empty id is a client error; no principal is unauthenticated).
func TestConfirmReceiptRejectsEmptyIDAndNoPrincipal(t *testing.T) {
	e := newReceiptEnv(t)
	_, err := e.h.ConfirmReceipt(incomingPrincipalCtx("buyer_1", "buyer"), &orderv1.ConfirmReceiptRequest{})
	wantReceiptCode(t, err, codes.InvalidArgument)
	o := e.order(t, store.OrderStatusShipped)
	_, err = e.h.ConfirmReceipt(context.Background(), &orderv1.ConfirmReceiptRequest{Id: o.ID})
	wantReceiptCode(t, err, codes.Unauthenticated)
	if e.statusOf(t, o.ID) != store.OrderStatusShipped {
		t.Fatal("order must be unchanged")
	}
}

// Scenario: Order receipt: Concurrent confirms emit one OrderCompleted
func TestConfirmReceiptConcurrentConfirmsEmitOneOrderCompleted(t *testing.T) {
	e := newReceiptEnv(t)
	o := e.order(t, store.OrderStatusShipped)
	const n = 8
	errs := make([]error, n)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, errs[i] = e.h.ConfirmReceipt(incomingPrincipalCtx("buyer_1", "buyer"), &orderv1.ConfirmReceiptRequest{Id: o.ID})
		}(i)
	}
	close(start)
	wg.Wait()
	wins, losers := 0, 0
	for _, err := range errs {
		switch status.Code(err) {
		case codes.OK:
			wins++
		case codes.FailedPrecondition:
			losers++
		default:
			t.Fatalf("unexpected error %v", err)
		}
	}
	if wins != 1 || losers != n-1 {
		t.Fatalf("want 1 success and %d FAILED_PRECONDITION, got %d and %d", n-1, wins, losers)
	}
	if got, _ := e.completedCount(t, o.ID); got != 1 {
		t.Fatalf("want one OrderCompleted, got %d", got)
	}
}

// Scenario: Order receipt: A completed order cannot be reopened or cancelled
func TestConfirmReceiptCompletedOrderCannotBeReopenedOrCancelled(t *testing.T) {
	e := newReceiptEnv(t)
	o := e.order(t, store.OrderStatusShipped)
	buyer := incomingPrincipalCtx("buyer_1", "buyer")
	if _, err := e.h.ConfirmReceipt(buyer, &orderv1.ConfirmReceiptRequest{Id: o.ID}); err != nil {
		t.Fatal(err)
	}
	_, err := e.h.CancelOrder(buyer, &orderv1.CancelOrderRequest{Id: o.ID})
	wantReceiptCode(t, err, codes.FailedPrecondition)
	_, err = e.h.UpdateOrderStatus(incomingPrincipalCtx("seller_1", "seller"), &orderv1.UpdateOrderStatusRequest{Id: o.ID, Status: orderv1.OrderStatus_ORDER_STATUS_SHIPPED})
	if err == nil {
		t.Fatal("reopening a completed order must fail")
	}
	if e.statusOf(t, o.ID) != store.OrderStatusCompleted {
		t.Fatal("order must stay COMPLETED")
	}
}

// Scenario: Order receipt: A buyer cannot complete through UpdateOrderStatus
// is task 5 / e2e; the handler keeps seller completion working and emitting.
// Scenario: Order receipt: Seller completion still works and emits OrderCompleted
func TestUpdateOrderStatusSellerCompletionEmitsSellerOrderCompleted(t *testing.T) {
	e := newReceiptEnv(t)
	o := e.order(t, store.OrderStatusShipped)
	if _, err := e.h.UpdateOrderStatus(incomingPrincipalCtx("seller_1", "seller"), &orderv1.UpdateOrderStatusRequest{Id: o.ID, Status: orderv1.OrderStatus_ORDER_STATUS_COMPLETED}); err != nil {
		t.Fatal(err)
	}
	if n, by := e.completedCount(t, o.ID); n != 1 || by[0] != orderv1.CompletedBy_COMPLETED_BY_SELLER {
		t.Fatalf("want one SELLER OrderCompleted, got %d %v", n, by)
	}
}
