package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/buidangphuc/team-order/internal/repository"
	"github.com/buidangphuc/team-order/internal/service"
	"github.com/buidangphuc/team-order/internal/upstream/upstreamtest"
)

const testTTL = time.Minute

type sweepRig struct {
	domain *upstreamtest.Domain
	orders *repository.InMemoryOrderRepository
	sagas  *repository.InMemorySagaRepository
	carts  *repository.InMemoryCartRepository
	svc    *service.OrderService
}

func newSweepRig(t *testing.T) *sweepRig {
	t.Helper()
	r := &sweepRig{
		domain: upstreamtest.NewDomain(map[string]int32{"lst_1": 10}),
		orders: repository.NewInMemoryOrderRepository(),
		sagas:  repository.NewInMemorySagaRepository(),
		carts:  repository.NewInMemoryCartRepository(),
	}
	r.svc = service.NewOrderService(r.orders, r.carts, nil, nil, r.domain, nil, nil,
		service.WithSagaRepository(r.sagas), service.WithReservationTTL(testTTL),
		service.WithReleaseRetry(time.Second, 1, time.Millisecond))
	if _, err := r.carts.AddItem(context.Background(), repository.CartItem{
		UserID: "buyer_1", ListingID: "lst_1", Quantity: 2, UnitPrice: 1000, SellerID: "s1",
	}); err != nil {
		t.Fatal(err)
	}
	return r
}

// A checkout attempt that crashed after reserving (saga PENDING, reservation
// RESERVED in team-order, active in team-domain) is settled once the TTL passes:
// the hold is released, the saga COMPENSATED and its key freed.
func TestSweep_SettlesAStalePendingSaga(t *testing.T) {
	r := newSweepRig(t)
	ctx := context.Background()
	sg, _, err := r.sagas.CreateSaga(ctx, repository.Saga{BuyerID: "buyer_1", IdempotencyKey: "K-crash"})
	if err != nil {
		t.Fatal(err)
	}
	item := repository.CartItem{ID: "ci_x", ListingID: "lst_1", Quantity: 2}
	res := repository.Reservation{ID: service.ReservationID(sg.ID, item), SagaID: sg.ID, BuyerID: "buyer_1",
		ListingID: "lst_1", Quantity: 2, Status: repository.ReservationStatusReserved, ExpiresAt: time.Now().Add(testTTL)}
	if _, err := r.sagas.CreateReservation(ctx, res); err != nil {
		t.Fatal(err)
	}
	if _, err := r.domain.ReserveStock(ctx, reserveReq(res)); err != nil {
		t.Fatal(err)
	}
	if r.domain.Stock("lst_1") != 8 {
		t.Fatal("setup: stock must be held")
	}

	// Before the TTL: a keyed retry is still "in progress".
	if _, err := r.svc.CreateOrdersFromCart(ctx, "buyer_1", addr(), nil, 1, "", service.WithIdempotencyKey("K-crash")); err == nil {
		t.Fatal("before the sweep the key is held by the crashed attempt")
	}
	if _, err := r.svc.SweepExpiredReservations(ctx, time.Now().Add(2*testTTL)); err != nil {
		t.Fatal(err)
	}
	if r.domain.Stock("lst_1") != 10 {
		t.Fatalf("the crashed attempt's hold must be released, stock=%d", r.domain.Stock("lst_1"))
	}
	if got, _ := r.sagas.GetSaga(ctx, sg.ID); got.Status != repository.SagaStatusCompensated || got.IdempotencyKey != "" {
		t.Fatalf("saga must be COMPENSATED with its key freed: %+v", got)
	}
	if got, err := r.svc.CreateOrdersFromCart(ctx, "buyer_1", addr(), nil, 1, "", service.WithIdempotencyKey("K-crash")); err != nil || len(got) != 1 {
		t.Fatalf("the freed key must run a fresh checkout: %v", err)
	}
}

// A cancel that crashed between its claim and its release leaves a Cancelled order
// whose reservation is still COMMITTED: the sweep releases it.
func TestSweep_ReleasesReservationsOfCancelledOrders(t *testing.T) {
	r := newSweepRig(t)
	ctx := context.Background()
	placed, err := r.svc.CreateOrdersFromCart(ctx, "buyer_1", addr(), nil, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	cancelWithoutRelease(t, r.orders, placed[0].ID)
	if r.domain.Stock("lst_1") != 8 {
		t.Fatal("setup: the crashed cancel released nothing")
	}
	// Not before a release timeout has passed since the cancel.
	if _, err := r.svc.SweepExpiredReservations(ctx, time.Now()); err != nil {
		t.Fatal(err)
	}
	if r.domain.Stock("lst_1") != 8 {
		t.Fatal("the sweep must not race a cancel that is still releasing")
	}
	n, err := r.svc.SweepExpiredReservations(ctx, time.Now().Add(2*time.Second))
	if err != nil || n != 1 {
		t.Fatalf("want 1 release, got %d %v", n, err)
	}
	if r.domain.Stock("lst_1") != 10 {
		t.Fatalf("stock %d, want 10", r.domain.Stock("lst_1"))
	}
	if n, _ := r.svc.SweepExpiredReservations(ctx, time.Now().Add(time.Hour)); n != 0 || r.domain.Stock("lst_1") != 10 {
		t.Fatalf("a second sweep must change nothing: %d released, stock %d", n, r.domain.Stock("lst_1"))
	}
}

// A completed checkout's reservations are never touched by the sweep, however
// late it runs.
func TestSweep_NeverTouchesACompletedSaga(t *testing.T) {
	r := newSweepRig(t)
	ctx := context.Background()
	if _, err := r.svc.CreateOrdersFromCart(ctx, "buyer_1", addr(), nil, 1, ""); err != nil {
		t.Fatal(err)
	}
	if n, err := r.svc.SweepExpiredReservations(ctx, time.Now().Add(24*time.Hour)); err != nil || n != 0 {
		t.Fatalf("sweep released %d (%v)", n, err)
	}
	if r.domain.Calls.Release != 0 || r.domain.Stock("lst_1") != 8 {
		t.Fatalf("release calls=%d stock=%d", r.domain.Calls.Release, r.domain.Stock("lst_1"))
	}
}
