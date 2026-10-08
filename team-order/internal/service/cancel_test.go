package service_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/team-order/internal/repository"
	"github.com/buidangphuc/team-order/internal/service"
	"github.com/buidangphuc/team-order/internal/upstream/upstreamtest"
)

type cancelRig struct {
	domain *upstreamtest.Domain
	orders *repository.InMemoryOrderRepository
	sagas  *repository.InMemorySagaRepository
	promo  *fakePromotion
	svc    *service.OrderService
}

// placeOrder checks out quantity 2 of lst_1 (stock 10 -> 8), optionally with a
// voucher, and returns the order.
func newCancelRig(t *testing.T, voucher string) (*cancelRig, repository.Order) {
	t.Helper()
	r := &cancelRig{
		domain: upstreamtest.NewDomain(map[string]int32{"lst_1": 10}),
		orders: repository.NewInMemoryOrderRepository(),
		sagas:  repository.NewInMemorySagaRepository(),
		promo:  newFakePromotion(500),
	}
	carts := repository.NewInMemoryCartRepository()
	if _, err := carts.AddItem(context.Background(), repository.CartItem{UserID: "buyer_1", ListingID: "lst_1", Quantity: 2, UnitPrice: 1000, SellerID: "s1"}); err != nil {
		t.Fatal(err)
	}
	r.svc = service.NewOrderService(r.orders, carts, nil, nil, r.domain, nil, nil,
		service.WithSagaRepository(r.sagas), service.WithPromotionClient(r.promo),
		service.WithReleaseRetry(time.Second, 2, time.Millisecond))
	placed, err := r.svc.CreateOrdersFromCart(context.Background(), "buyer_1", addr(), nil, 1, voucher)
	if err != nil || len(placed) != 1 {
		t.Fatalf("checkout: %v", err)
	}
	if r.domain.Stock("lst_1") != 8 {
		t.Fatalf("setup stock %d", r.domain.Stock("lst_1"))
	}
	return r, placed[0]
}

func (r *cancelRig) setStatus(t *testing.T, id string, to repository.OrderStatus, from ...repository.OrderStatus) {
	t.Helper()
	if _, err := r.orders.UpdateOrderStatusFrom(context.Background(), id, to, from, ""); err != nil {
		t.Fatal(err)
	}
}

func TestCancel_ConcurrentCancelsReleaseOnce(t *testing.T) {
	r, o := newCancelRig(t, "")
	const n = 6
	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, errs[i] = r.svc.CancelOrder(context.Background(), o.ID)
		}(i)
	}
	close(start)
	wg.Wait()
	wins := 0
	for _, err := range errs {
		switch {
		case err == nil:
			wins++
		case !errors.Is(err, service.ErrInvalidStatus):
			t.Fatalf("losers must get ErrInvalidStatus, got %v", err)
		}
	}
	if wins != 1 {
		t.Fatalf("exactly one cancel must win, got %d", wins)
	}
	if r.domain.Stock("lst_1") != 10 {
		t.Fatalf("stock %d, want 10", r.domain.Stock("lst_1"))
	}
	if r.domain.Calls.Release != 1 {
		t.Fatalf("only the winner releases: %d release calls", r.domain.Calls.Release)
	}
}

func TestCancel_PaidOrderRestoresStock(t *testing.T) {
	r, o := newCancelRig(t, "")
	r.setStatus(t, o.ID, repository.OrderStatusPaid, repository.OrderStatusPending)
	res, err := r.svc.CancelOrder(context.Background(), o.ID)
	if err != nil || res.Status != repository.OrderStatusCancelled || res.ReleasePending {
		t.Fatalf("cancel paid: %v %+v", err, res)
	}
	if r.domain.Stock("lst_1") != 10 {
		t.Fatalf("stock %d, want 10", r.domain.Stock("lst_1"))
	}
}

func TestCancel_ShippedOrderIsRefusedAndReleasesNothing(t *testing.T) {
	r, o := newCancelRig(t, "VOUCHER")
	r.setStatus(t, o.ID, repository.OrderStatusShipped, repository.OrderStatusPending)
	if _, err := r.svc.CancelOrder(context.Background(), o.ID); !errors.Is(err, service.ErrInvalidStatus) {
		t.Fatalf("want ErrInvalidStatus, got %v", err)
	}
	got, _ := r.orders.GetOrder(context.Background(), o.ID)
	if got.Status != repository.OrderStatusShipped || r.domain.Stock("lst_1") != 8 || r.domain.Calls.Release != 0 {
		t.Fatalf("status=%v stock=%d releases=%d", got.Status, r.domain.Stock("lst_1"), r.domain.Calls.Release)
	}
	if len(r.promo.releasedIDs) != 0 {
		t.Fatalf("a refused cancel must not release the voucher: %v", r.promo.releasedIDs)
	}
}

func TestCancel_FailingReleaseIsParkedAndSwept(t *testing.T) {
	r, o := newCancelRig(t, "")
	r.domain.ReleaseErr = func(string) error { return status.Error(codes.Unavailable, "team-domain stopped") }
	res, err := r.svc.CancelOrder(context.Background(), o.ID)
	if err != nil || res.Status != repository.OrderStatusCancelled || !res.ReleasePending {
		t.Fatalf("the cancel must succeed with the release parked: %v %+v", err, res)
	}
	held, _ := r.sagas.ListReservationsByOrder(context.Background(), o.ID)
	if len(held) != 1 || held[0].Status != repository.ReservationStatusReleaseFailed {
		t.Fatalf("release must be parked RELEASE_FAILED: %+v", held)
	}
	r.domain.ReleaseErr = nil // team-domain is back
	if n, err := r.svc.SweepExpiredReservations(context.Background(), time.Now().Add(2*time.Second)); err != nil || n != 1 {
		t.Fatalf("sweep: %d %v", n, err)
	}
	if r.domain.Stock("lst_1") != 10 {
		t.Fatalf("stock %d, want 10", r.domain.Stock("lst_1"))
	}
	if _, err := r.svc.SweepExpiredReservations(context.Background(), time.Now().Add(time.Hour)); err != nil || r.domain.Stock("lst_1") != 10 {
		t.Fatalf("stock must stay 10, got %d", r.domain.Stock("lst_1"))
	}
}

func TestCancel_ReleasesTheVoucherOnceAndToleratesItsFailure(t *testing.T) {
	r, o := newCancelRig(t, "VOUCHER")
	if _, err := r.svc.CancelOrder(context.Background(), o.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.CancelOrder(context.Background(), o.ID); !errors.Is(err, service.ErrInvalidStatus) {
		t.Fatalf("second cancel: %v", err)
	}
	if len(r.promo.releasedIDs) != 1 || r.promo.releasedIDs[0] != o.ID {
		t.Fatalf("voucher hold must be released once under the order id: %v", r.promo.releasedIDs)
	}

	r2, o2 := newCancelRig(t, "VOUCHER")
	r2.promo.releaseErr = errors.New("promotion down")
	res, err := r2.svc.CancelOrder(context.Background(), o2.ID)
	if err != nil || res.Status != repository.OrderStatusCancelled || r2.domain.Stock("lst_1") != 10 {
		t.Fatalf("a voucher release error must not fail the cancel: %v %+v", err, res)
	}
}
