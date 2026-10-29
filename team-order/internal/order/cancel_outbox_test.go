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
	"github.com/buidangphuc/team-order/internal/store/fake"
	"github.com/buidangphuc/team-order/internal/upstream/upstreamtest"
)

// cancelFacts decodes the OrderCancelled rows the outbox holds for an order.
func cancelFacts(t *testing.T, outbox *pgOutbox, orderID string) []*orderv1.OrderCancelled {
	t.Helper()
	var out []*orderv1.OrderCancelled
	for _, r := range outbox.EnqueuedRows() {
		if r.AggregateID != orderID || r.EventType != OrderCancelledEventType {
			continue
		}
		var env eventsv1.EventEnvelope
		if err := proto.Unmarshal(r.Payload, &env); err != nil {
			t.Fatal(err)
		}
		var ev orderv1.OrderCancelled
		if err := proto.Unmarshal(env.GetPayload(), &ev); err != nil {
			t.Fatal(err)
		}
		out = append(out, &ev)
	}
	return out
}

func newCancelOutboxRig(t *testing.T) (*order.OrderService, *store.PostgresOrderRepository, *pgOutbox, store.Order) {
	t.Helper()
	outbox := newPGOutbox(t)
	orders := outbox.orders(store.WithPaidOutbox(BuildPaidOutboxRow),
		store.WithCancelledOutbox(BuildCancelledOutboxRow))
	carts := fake.NewInMemoryCartRepository()
	if _, err := carts.AddItem(context.Background(), store.CartItem{UserID: "buyer_1", ListingID: "lst_1", Quantity: 2, UnitPrice: 1000, SellerID: "s1"}); err != nil {
		t.Fatal(err)
	}
	svc := newSvc(t, orders, carts, nil, nil, upstreamtest.NewDomain(map[string]int32{"lst_1": 10}), nil, nil,
		order.WithSagaRepository(outbox.sagas()), order.WithReleaseRetry(time.Second, 1, time.Millisecond))
	placed, err := svc.CreateOrdersFromCart(context.Background(), "buyer_1", addr(), nil, 1, "")
	if err != nil || len(placed) != 1 {
		t.Fatalf("checkout: %v", err)
	}
	return svc, orders, outbox, placed[0]
}

// CancelOrder emits OrderCancelled through its claim: once, with the status the
// order was cancelled from; a second (lost) cancel emits nothing.
func TestCancelOrder_EmitsCancelledFactThroughTheClaim(t *testing.T) {
	for _, tc := range []struct {
		name string
		pay  bool
		want orderv1.OrderStatus
	}{
		{"pending", false, orderv1.OrderStatus_ORDER_STATUS_PENDING},
		{"paid", true, orderv1.OrderStatus_ORDER_STATUS_PAID},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, orders, outbox, o := newCancelOutboxRig(t)
			if tc.pay {
				if _, err := orders.UpdateOrderStatusFrom(context.Background(), o.ID, store.OrderStatusPaid,
					[]store.OrderStatus{store.OrderStatusPending}, ""); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := svc.CancelOrder(context.Background(), o.ID); err != nil {
				t.Fatal(err)
			}
			if _, err := svc.CancelOrder(context.Background(), o.ID); !errors.Is(err, order.ErrInvalidStatus) {
				t.Fatalf("second cancel: want ErrInvalidStatus, got %v", err)
			}
			facts := cancelFacts(t, outbox, o.ID)
			if len(facts) != 1 {
				t.Fatalf("want 1 OrderCancelled, got %d", len(facts))
			}
			if f := facts[0]; f.GetPreviousStatus() != tc.want || f.GetBuyerId() != "buyer_1" || f.GetSellerId() != "s1" ||
				f.GetTotalAmount() != o.TotalAmount {
				t.Fatalf("fact: %+v", f)
			}
		})
	}
}

// A cancel that loses to a later status (Shipped) is refused and emits nothing.
func TestCancelOrder_RefusedCancelEmitsNothing(t *testing.T) {
	svc, orders, outbox, o := newCancelOutboxRig(t)
	ctx := context.Background()
	if _, err := orders.UpdateOrderStatusFrom(ctx, o.ID, store.OrderStatusPaid, []store.OrderStatus{store.OrderStatusPending}, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := orders.UpdateOrderStatusFrom(ctx, o.ID, store.OrderStatusShipped, []store.OrderStatus{store.OrderStatusPaid}, "TRK"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CancelOrder(ctx, o.ID); !errors.Is(err, order.ErrInvalidStatus) {
		t.Fatalf("want ErrInvalidStatus, got %v", err)
	}
	if n := len(cancelFacts(t, outbox, o.ID)); n != 0 {
		t.Fatalf("want 0 OrderCancelled, got %d", n)
	}
}
