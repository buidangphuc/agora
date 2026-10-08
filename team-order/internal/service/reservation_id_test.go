package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/buidangphuc/team-order/internal/repository"
	"github.com/buidangphuc/team-order/internal/service"
	"github.com/buidangphuc/team-order/internal/upstream/upstreamtest"
)

func TestReservationID_ScopedToTheAttempt(t *testing.T) {
	item := repository.CartItem{ID: "ci_1", ListingID: "lst_1", Quantity: 2}
	a := service.ReservationID("saga_1", item)
	if a == "" || a != service.ReservationID("saga_1", item) {
		t.Fatalf("the same attempt and item must map to one stable id, got %q", a)
	}
	if a == service.ReservationID("saga_2", item) {
		t.Fatal("two attempts of one cart item must never share a reservation id")
	}
	if a == service.ReservationID("saga_1", repository.CartItem{ID: "ci_2", ListingID: "lst_1", Quantity: 2}) {
		t.Fatal("different cart items must get different reservation ids")
	}
	if a == service.ReservationID("saga_1", repository.CartItem{ID: "ci_1", ListingID: "lst_1", Quantity: 3}) {
		t.Fatal("a changed quantity must get a different reservation id")
	}
}

// An unkeyed checkout that is compensated (second item out of stock) and then
// retried after a restock reserves under NEW ids: team-domain refuses to re-reserve
// a released id, so a buyer-scoped id would fail the retry forever. Stock of the
// first listing ends reduced exactly once.
func TestUnkeyedRetryAfterCompensationReservesAgainAndDecrementsOnce(t *testing.T) {
	ctx := context.Background()
	domain := upstreamtest.NewDomain(map[string]int32{"lst_a": 10, "lst_b": 0})
	orders := repository.NewInMemoryOrderRepository()
	saga := &recordingSagaRepo{SagaRepository: repository.NewInMemorySagaRepository()}
	cart := &fakeCartRepo{items: []repository.CartItem{
		{ID: "ci_a", ListingID: "lst_a", Quantity: 2, SellerID: "s1", UnitPrice: 1000},
		{ID: "ci_b", ListingID: "lst_b", Quantity: 1, SellerID: "s1", UnitPrice: 1000},
	}}
	svc := service.NewOrderService(orders, cart, nil, nil, domain, nil, nil, service.WithSagaRepository(saga))

	if _, err := svc.CreateOrdersFromCart(ctx, "buyer_1", addr(), nil, 1, ""); !errors.Is(err, service.ErrInsufficientStock) {
		t.Fatalf("first attempt: want ErrInsufficientStock, got %v", err)
	}
	if got := domain.Stock("lst_a"); got != 10 {
		t.Fatalf("compensation must return lst_a's stock, got %d", got)
	}

	domain.SetStock("lst_b", 5) // seller restocks
	placed, err := svc.CreateOrdersFromCart(ctx, "buyer_1", addr(), nil, 1, "")
	if err != nil {
		t.Fatalf("retry must succeed, got %v", err)
	}
	if len(placed) != 1 {
		t.Fatalf("want 1 order, got %d", len(placed))
	}
	if got := domain.Stock("lst_a"); got != 8 {
		t.Fatalf("lst_a must be reduced once (8), got %d", got)
	}
	if ids := saga.sagaIDs(); len(ids) != 2 || ids[0] == ids[1] {
		t.Fatalf("each attempt must open its own saga, got %v", ids)
	}
}
