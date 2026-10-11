package service_test

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/team-order/internal/repository"
	"github.com/buidangphuc/team-order/internal/service"
	"github.com/buidangphuc/team-order/internal/upstream/upstreamtest"
)

type checkoutRig struct {
	domain *upstreamtest.Domain
	orders *repository.InMemoryOrderRepository
	sagas  *recordingSagaRepo
	cart   *fakeCartRepo
	svc    *service.OrderService
}

// twoSellerRig: seller sa sells lst_a (stock 10), seller sb sells lst_b (stock
// stockB); the cart holds 2 x lst_a and 1 x lst_b.
func twoSellerRig(t *testing.T, stockB int32, opts ...service.OrderServiceOption) *checkoutRig {
	t.Helper()
	r := &checkoutRig{
		domain: upstreamtest.NewDomain(map[string]int32{"lst_a": 10, "lst_b": stockB}),
		orders: repository.NewInMemoryOrderRepository(),
		sagas:  &recordingSagaRepo{SagaRepository: repository.NewInMemorySagaRepository()},
		cart: &fakeCartRepo{items: []repository.CartItem{
			{ID: "ci_b", ListingID: "lst_b", Quantity: 1, SellerID: "sb", UnitPrice: 2000},
			{ID: "ci_a", ListingID: "lst_a", Quantity: 2, SellerID: "sa", UnitPrice: 1000},
		}},
	}
	inner := r.sagas.SagaRepository.(*repository.InMemorySagaRepository)
	all := append([]service.OrderServiceOption{
		service.WithSagaRepository(r.sagas),
		service.WithOrderPlacer(repository.NewInMemoryOrderPlacer(r.orders, inner)),
	}, opts...)
	r.svc = service.NewOrderService(r.orders, r.cart, nil, nil, r.domain, nil, nil, all...)
	return r
}

func (r *checkoutRig) buyerOrders(t *testing.T) []repository.Order {
	t.Helper()
	os, err := r.orders.ListBuyerOrders(context.Background(), "buyer_1", 0)
	if err != nil {
		t.Fatal(err)
	}
	return os
}

func (r *checkoutRig) sagaStatus(t *testing.T) repository.SagaStatus {
	t.Helper()
	res, err := r.sagas.ListReservationsBySaga(context.Background(), r.sagas.lastSagaID())
	if err != nil || len(res) == 0 {
		t.Fatalf("no reservations for saga: %v", err)
	}
	sg, err := r.sagas.GetSaga(context.Background(), r.sagas.lastSagaID())
	if err != nil {
		t.Fatal(err)
	}
	return sg.Status
}

func TestCheckout_SecondSellerOutOfStock_PlacesNothing(t *testing.T) {
	r := twoSellerRig(t, 0)
	_, err := r.svc.CreateOrdersFromCart(context.Background(), "buyer_1", addr(), nil, 1, "")
	if !errors.Is(err, service.ErrInsufficientStock) {
		t.Fatalf("want ErrInsufficientStock, got %v", err)
	}
	if got := r.buyerOrders(t); len(got) != 0 {
		t.Fatalf("no order may exist for any seller, got %d", len(got))
	}
	if got := r.domain.Stock("lst_a"); got != 10 {
		t.Fatalf("seller a's hold must be released, stock=%d", got)
	}
	if r.cart.removeCalled {
		t.Fatal("the cart must stay intact")
	}
	if st := r.sagaStatus(t); st != repository.SagaStatusCompensated {
		t.Fatalf("saga status %v, want COMPENSATED", st)
	}
}

func TestCheckout_TwoSellers_OneOrderEach(t *testing.T) {
	r := twoSellerRig(t, 5)
	got, err := r.svc.CreateOrdersFromCart(context.Background(), "buyer_1", addr(), nil, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].SellerID != "sa" || got[1].SellerID != "sb" {
		t.Fatalf("want one order per seller in sorted order, got %+v", got)
	}
	for _, o := range got {
		if o.Status != repository.OrderStatusPending {
			t.Fatalf("order %s status %v", o.ID, o.Status)
		}
	}
	if r.domain.Stock("lst_a") != 8 || r.domain.Stock("lst_b") != 4 {
		t.Fatalf("stock a=%d b=%d, want 8/4", r.domain.Stock("lst_a"), r.domain.Stock("lst_b"))
	}
	if !r.cart.removeCalled || len(r.cart.removedItemID) != 2 {
		t.Fatalf("checked-out items must leave the cart: %v", r.cart.removedItemID)
	}
	res, _ := r.sagas.ListReservationsBySaga(context.Background(), r.sagas.lastSagaID())
	for _, x := range res {
		if x.Status != repository.ReservationStatusCommitted || x.OrderID == "" {
			t.Fatalf("reservation %+v must be bound COMMITTED", x)
		}
		if r.domain.State(x.ID) != upstreamtest.StateCommitted {
			t.Fatalf("reservation %s must be committed in team-domain, is %q", x.ID, r.domain.State(x.ID))
		}
	}
	if st := r.sagaStatus(t); st != repository.SagaStatusCompleted {
		t.Fatalf("saga status %v, want COMPLETED", st)
	}
	// A placed order's stock survives team-domain's TTL sweep.
	if n := r.domain.Sweep(); n != 0 || r.domain.Stock("lst_a") != 8 {
		t.Fatalf("sweep restored %d reservations, stock a=%d", n, r.domain.Stock("lst_a"))
	}
}

func TestCheckout_CommitRefused_PlacesNothing(t *testing.T) {
	r := twoSellerRig(t, 5)
	// team-domain swept the holds between reserve and commit.
	r.domain.CommitErr = func(string) error {
		r.domain.CommitErr = nil
		r.domain.Sweep()
		return nil
	}
	_, err := r.svc.CreateOrdersFromCart(context.Background(), "buyer_1", addr(), nil, 1, "")
	if !errors.Is(err, service.ErrReservationLost) {
		t.Fatalf("want ErrReservationLost, got %v", err)
	}
	if got := r.buyerOrders(t); len(got) != 0 {
		t.Fatalf("no order may be placed, got %d", len(got))
	}
	if r.domain.Stock("lst_a") != 10 || r.domain.Stock("lst_b") != 5 {
		t.Fatalf("stock must be back exactly once: a=%d b=%d", r.domain.Stock("lst_a"), r.domain.Stock("lst_b"))
	}
	if r.cart.removeCalled {
		t.Fatal("the cart must stay intact")
	}
}

func TestCheckout_CommitUnimplemented_IsAFailure(t *testing.T) {
	r := twoSellerRig(t, 5)
	r.domain.CommitErr = func(string) error { return status.Error(codes.Unimplemented, "old domain") }
	if _, err := r.svc.CreateOrdersFromCart(context.Background(), "buyer_1", addr(), nil, 1, ""); err == nil {
		t.Fatal("an UNIMPLEMENTED commit must fail the checkout")
	}
	if got := r.buyerOrders(t); len(got) != 0 {
		t.Fatalf("no order may be placed, got %d", len(got))
	}
	if r.domain.Stock("lst_a") != 10 || r.domain.Stock("lst_b") != 5 {
		t.Fatal("holds must be released")
	}
}

// faultyPlacer runs the real placer and then reports an error (a lost commit
// acknowledgement), or fails without placing anything, or places only the first
// order.
type faultyPlacer struct {
	inner repository.OrderPlacer
	mode  string // "after-commit", "before", "partial"
}

func (p faultyPlacer) PlaceOrders(ctx context.Context, sagaID string, placed []repository.PlacedOrder) ([]repository.Order, error) {
	switch p.mode {
	case "after-commit":
		if _, err := p.inner.PlaceOrders(ctx, sagaID, placed); err != nil {
			return nil, err
		}
	case "partial":
		if _, err := p.inner.PlaceOrders(ctx, sagaID, placed[:1]); err != nil {
			return nil, err
		}
	}
	return nil, errors.New("connection reset while committing")
}

func TestCheckout_AmbiguousPlacement(t *testing.T) {
	for _, tc := range []struct {
		mode        string
		wantErr     error
		wantOrders  int
		wantStockA  int32
		wantRelease bool
	}{
		{"after-commit", nil, 2, 8, false},
		{"before", nil, 0, 10, true},
		{"partial", service.ErrPlacementUnknown, 1, 8, false},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			orders := repository.NewInMemoryOrderRepository()
			inner := repository.NewInMemorySagaRepository()
			r := twoSellerRig(t, 5)
			r.orders = orders
			r.sagas = &recordingSagaRepo{SagaRepository: inner}
			r.svc = service.NewOrderService(orders, r.cart, nil, nil, r.domain, nil, nil,
				service.WithSagaRepository(r.sagas),
				service.WithOrderPlacer(faultyPlacer{inner: repository.NewInMemoryOrderPlacer(orders, inner), mode: tc.mode}))

			got, err := r.svc.CreateOrdersFromCart(context.Background(), "buyer_1", addr(), nil, 1, "")
			switch {
			case tc.mode == "after-commit":
				if err != nil || len(got) != 2 {
					t.Fatalf("orders that exist must be returned as placed: %v %d", err, len(got))
				}
			case tc.wantErr != nil:
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("want %v, got %v", tc.wantErr, err)
				}
			default:
				if err == nil {
					t.Fatal("want an error")
				}
			}
			if n := len(r.buyerOrders(t)); n != tc.wantOrders {
				t.Fatalf("orders=%d want %d", n, tc.wantOrders)
			}
			if s := r.domain.Stock("lst_a"); s != tc.wantStockA {
				t.Fatalf("stock a=%d want %d", s, tc.wantStockA)
			}
			if released := r.domain.Calls.Release > 0; released != tc.wantRelease {
				t.Fatalf("release calls=%d, want release=%v", r.domain.Calls.Release, tc.wantRelease)
			}
		})
	}
}
