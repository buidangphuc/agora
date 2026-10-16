package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/buidangphuc/team-order/internal/repository"
	"github.com/buidangphuc/team-order/internal/service"
)

var allStatuses = []repository.OrderStatus{
	repository.OrderStatusPending, repository.OrderStatusPaid, repository.OrderStatusShipped,
	repository.OrderStatusCompleted, repository.OrderStatusCancelled,
}

// legal is the spec table (order-lifecycle-guards), written out independently of
// the implementation.
func legal(from, to repository.OrderStatus, actor service.Actor) bool {
	type k struct {
		from, to repository.OrderStatus
		a        service.Actor
	}
	table := map[k]bool{
		{repository.OrderStatusPending, repository.OrderStatusPaid, service.ActorSystem}:      true,
		{repository.OrderStatusPending, repository.OrderStatusShipped, service.ActorSeller}:   true,
		{repository.OrderStatusPaid, repository.OrderStatusShipped, service.ActorSeller}:      true,
		{repository.OrderStatusShipped, repository.OrderStatusCompleted, service.ActorSeller}: true,
		// Order receipt: buyer (ConfirmReceipt) and system (auto-complete sweep) complete from Shipped only
		{repository.OrderStatusShipped, repository.OrderStatusCompleted, service.ActorBuyer}:  true,
		{repository.OrderStatusShipped, repository.OrderStatusCompleted, service.ActorSystem}: true,
		{repository.OrderStatusPending, repository.OrderStatusCancelled, service.ActorBuyer}:  true,
		{repository.OrderStatusPaid, repository.OrderStatusCancelled, service.ActorBuyer}:     true,
		// Order expiry: System may cancel only from Pending
		{repository.OrderStatusPending, repository.OrderStatusCancelled, service.ActorSystem}: true,
	}
	return table[k{from, to, actor}]
}

func TestAllowedFrom_MatchesTheTableForEveryPair(t *testing.T) {
	for _, actor := range []service.Actor{service.ActorNone, service.ActorBuyer, service.ActorSeller, service.ActorSystem} {
		for _, to := range allStatuses {
			allowed := map[repository.OrderStatus]bool{}
			for _, f := range service.AllowedFrom(to, actor) {
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
			orders := repository.NewInMemoryOrderRepository()
			if _, err := orders.CreateOrder(ctx, repository.Order{ID: "o", BuyerID: "b", SellerID: "s", Status: from}); err != nil {
				t.Fatal(err)
			}
			svc := service.NewOrderService(orders, nil, nil, nil, nil, nil, nil)
			_, err := svc.UpdateOrderStatus(ctx, "o", service.ActorSeller, to, "")
			sellerMayRequest := to != repository.OrderStatusCancelled && len(service.AllowedFrom(to, service.ActorSeller)) > 0
			switch {
			case !sellerMayRequest:
				if !errors.Is(err, service.ErrActorForbidden) {
					t.Errorf("%v -> %v: want ErrActorForbidden, got %v", from, to, err)
				}
			case legal(from, to, service.ActorSeller):
				if err != nil {
					t.Errorf("%v -> %v: want success, got %v", from, to, err)
				}
			default:
				if !errors.Is(err, service.ErrInvalidStatus) {
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
	shipped := []repository.OrderStatus{repository.OrderStatusShipped}
	for _, actor := range []service.Actor{service.ActorBuyer, service.ActorSystem, service.ActorSeller} {
		got := service.AllowedFrom(repository.OrderStatusCompleted, actor)
		if len(got) != 1 || got[0] != shipped[0] {
			t.Errorf("AllowedFrom(Completed, %v) = %v, want exactly %v", actor, got, shipped)
		}
	}
	for _, actor := range []service.Actor{service.ActorNone, service.ActorBuyer, service.ActorSeller, service.ActorSystem} {
		for _, from := range service.AllowedFrom(repository.OrderStatusCompleted, actor) {
			if from == repository.OrderStatusPaid || from == repository.OrderStatusPending {
				t.Errorf("actor %v may complete from %v", actor, from)
			}
		}
	}
	if got := service.AllowedFrom(repository.OrderStatusCompleted, service.ActorNone); len(got) != 0 {
		t.Errorf("ActorNone may complete from %v", got)
	}
}

// Scenario: Order receipt: A buyer cannot complete through UpdateOrderStatus.
// The buyer row exists in the table only for ConfirmReceipt; UpdateOrderStatus
// with the buyer class stays forbidden and the order stays SHIPPED.
func TestUpdateOrderStatus_BuyerCannotComplete(t *testing.T) {
	ctx := context.Background()
	orders := repository.NewInMemoryOrderRepository()
	if _, err := orders.CreateOrder(ctx, repository.Order{ID: "o", BuyerID: "b", SellerID: "s", Status: repository.OrderStatusShipped}); err != nil {
		t.Fatal(err)
	}
	svc := service.NewOrderService(orders, nil, nil, nil, nil, nil, nil)
	_, err := svc.UpdateOrderStatus(ctx, "o", service.ActorBuyer, repository.OrderStatusCompleted, "")
	if !errors.Is(err, service.ErrActorForbidden) {
		t.Errorf("want ErrActorForbidden, got %v", err)
	}
	got, _ := orders.GetOrder(ctx, "o")
	if got.Status != repository.OrderStatusShipped {
		t.Errorf("order is %v, want Shipped", got.Status)
	}
}
