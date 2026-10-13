package order_test

import (
	"context"
	"errors"
	"testing"

	"github.com/buidangphuc/team-order/internal/order"
	"github.com/buidangphuc/team-order/internal/store"
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
		{store.OrderStatusPending, store.OrderStatusCancelled, order.ActorBuyer}:  true,
		{store.OrderStatusPaid, store.OrderStatusCancelled, order.ActorBuyer}:     true,
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
			orders := store.NewInMemoryOrderRepository()
			if _, err := orders.CreateOrder(ctx, store.Order{ID: "o", BuyerID: "b", SellerID: "s", Status: from}); err != nil {
				t.Fatal(err)
			}
			svc := order.NewOrderService(orders, nil, nil, nil, nil, nil, nil)
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
