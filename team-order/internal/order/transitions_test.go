package order_test

import (
	"context"
	"errors"
	"testing"

	"github.com/buidangphuc/team-order/internal/order"
	"github.com/buidangphuc/team-order/internal/store"
	"github.com/buidangphuc/team-order/internal/store/fake"
)

var allStatuses = []store.OrderStatus{
	store.OrderStatusPending, store.OrderStatusPaid, store.OrderStatusShipped,
	store.OrderStatusCompleted, store.OrderStatusCancelled,
}

// legal is the spec table (order-lifecycle-guards), written out independently of
// the implementation.
func legal(from, to store.OrderStatus, actor order.Actor) bool {
	type k struct {
		from, to store.OrderStatus
		a        order.Actor
	}
	table := map[k]bool{
		{store.OrderStatusPending, store.OrderStatusPaid, order.ActorSystem}:      true,
		{store.OrderStatusPending, store.OrderStatusShipped, order.ActorSeller}:   true,
		{store.OrderStatusPaid, store.OrderStatusShipped, order.ActorSeller}:      true,
		{store.OrderStatusShipped, store.OrderStatusCompleted, order.ActorSeller}: true,
		// Order receipt: buyer (ConfirmReceipt) and system (auto-complete sweep) complete from Shipped only
		{store.OrderStatusShipped, store.OrderStatusCompleted, order.ActorBuyer}:  true,
		{store.OrderStatusShipped, store.OrderStatusCompleted, order.ActorSystem}: true,
		{store.OrderStatusPending, store.OrderStatusCancelled, order.ActorBuyer}:  true,
		{store.OrderStatusPaid, store.OrderStatusCancelled, order.ActorBuyer}:     true,
		// Order expiry: System may cancel only from Pending
		{store.OrderStatusPending, store.OrderStatusCancelled, order.ActorSystem}: true,
	}
	return table[k{from, to, actor}]
}

func TestAllowedFrom_MatchesTheTableForEveryPair(t *testing.T) {
	for _, actor := range []order.Actor{order.ActorNone, order.ActorBuyer, order.ActorSeller, order.ActorSystem} {
		for _, to := range allStatuses {
			allowed := map[store.OrderStatus]bool{}
			for _, f := range order.AllowedFrom(to, actor) {
				allowed[f] = true
			}
			for _, from := range allStatuses {
				if allowed[from] != legal(from, to, actor) {
					t.Errorf("%v -> %v by %v: allowed=%v want %v", from, to, actor, allowed[from], legal(from, to, actor))
				}
			}
		}
	}
}

// Every (from, to) through the UpdateOrderStatus path for the seller class: a
// target the class may never request is ErrActorForbidden, a permitted target
// from the wrong status is ErrInvalidStatus, and a rejected change leaves the
// order unchanged.
func TestUpdateOrderStatus_EveryPairForTheSeller(t *testing.T) {
	ctx := context.Background()
	for _, from := range allStatuses {
		for _, to := range allStatuses {
			orders := fake.NewInMemoryOrderRepository()
			if _, err := orders.CreateOrder(ctx, store.Order{ID: "o", BuyerID: "b", SellerID: "s", Status: from}); err != nil {
				t.Fatal(err)
			}
			svc := newSvc(t, orders, nil, nil, nil, nil, nil, nil)
			_, err := svc.UpdateOrderStatus(ctx, "o", order.ActorSeller, to, "")
			sellerMayRequest := to != store.OrderStatusCancelled && len(order.AllowedFrom(to, order.ActorSeller)) > 0
			switch {
			case !sellerMayRequest:
				if !errors.Is(err, order.ErrActorForbidden) {
					t.Errorf("%v -> %v: want ErrActorForbidden, got %v", from, to, err)
				}
			case legal(from, to, order.ActorSeller):
				if err != nil {
					t.Errorf("%v -> %v: want success, got %v", from, to, err)
				}
			default:
				if !errors.Is(err, order.ErrInvalidStatus) {
					t.Errorf("%v -> %v: want ErrInvalidStatus, got %v", from, to, err)
				}
			}
			got, _ := orders.GetOrder(ctx, "o")
			want := from
			if err == nil {
				want = to
			}
			if got.Status != want {
				t.Errorf("%v -> %v: order is %v, want %v", from, to, got.Status, want)
			}
		}
	}
}

// Scenario: Order receipt: The table allows buyer and system to complete only from shipped.
func TestTransitionsCompletedActors(t *testing.T) {
	shipped := []store.OrderStatus{store.OrderStatusShipped}
	for _, actor := range []order.Actor{order.ActorBuyer, order.ActorSystem, order.ActorSeller} {
		got := order.AllowedFrom(store.OrderStatusCompleted, actor)
		if len(got) != 1 || got[0] != shipped[0] {
			t.Errorf("AllowedFrom(Completed, %v) = %v, want exactly %v", actor, got, shipped)
		}
	}
	for _, actor := range []order.Actor{order.ActorNone, order.ActorBuyer, order.ActorSeller, order.ActorSystem} {
		for _, from := range order.AllowedFrom(store.OrderStatusCompleted, actor) {
			if from == store.OrderStatusPaid || from == store.OrderStatusPending {
				t.Errorf("actor %v may complete from %v", actor, from)
			}
		}
	}
	if got := order.AllowedFrom(store.OrderStatusCompleted, order.ActorNone); len(got) != 0 {
		t.Errorf("ActorNone may complete from %v", got)
	}
}

// Scenario: Order receipt: A buyer cannot complete through UpdateOrderStatus.
// The buyer row exists in the table only for ConfirmReceipt; UpdateOrderStatus
// with the buyer class stays forbidden and the order stays SHIPPED.
func TestUpdateOrderStatus_BuyerCannotComplete(t *testing.T) {
	ctx := context.Background()
	orders := fake.NewInMemoryOrderRepository()
	if _, err := orders.CreateOrder(ctx, store.Order{ID: "o", BuyerID: "b", SellerID: "s", Status: store.OrderStatusShipped}); err != nil {
		t.Fatal(err)
	}
	svc := newSvc(t, orders, nil, nil, nil, nil, nil, nil)
	_, err := svc.UpdateOrderStatus(ctx, "o", order.ActorBuyer, store.OrderStatusCompleted, "")
	if !errors.Is(err, order.ErrActorForbidden) {
		t.Errorf("want ErrActorForbidden, got %v", err)
	}
	got, _ := orders.GetOrder(ctx, "o")
	if got.Status != store.OrderStatusShipped {
		t.Errorf("order is %v, want Shipped", got.Status)
	}
}
