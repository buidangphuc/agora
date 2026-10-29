package order_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"

	paymentv1 "github.com/buidangphuc/team-order/generated/platform/payment/v1"
	promotionv1 "github.com/buidangphuc/team-order/generated/platform/promotion/v1"
	"github.com/buidangphuc/team-order/internal/order"
	"github.com/buidangphuc/team-order/internal/store"
	"github.com/buidangphuc/team-order/internal/store/fake"
	"github.com/buidangphuc/team-order/internal/upstream/upstreamtest"
)

type platPromo struct {
	mu      sync.Mutex
	fail    bool
	release []string
}

func (p *platPromo) ValidateAndReserve(context.Context, *promotionv1.ValidateAndReserveRequest, ...grpc.CallOption) (*promotionv1.ValidateAndReserveResponse, error) {
	return &promotionv1.ValidateAndReserveResponse{Valid: false, Reason: "unused"}, nil
}

func (p *platPromo) CommitReservation(context.Context, *promotionv1.CommitReservationRequest, ...grpc.CallOption) (*promotionv1.CommitReservationResponse, error) {
	return &promotionv1.CommitReservationResponse{Committed: true}, nil
}

func (p *platPromo) ReleaseReservation(_ context.Context, in *promotionv1.ReleaseReservationRequest, _ ...grpc.CallOption) (*promotionv1.ReleaseReservationResponse, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.release = append(p.release, in.GetReservationId())
	if p.fail {
		return nil, errors.New("promotion unavailable")
	}
	return &promotionv1.ReleaseReservationResponse{Released: true}, nil
}

func (p *platPromo) platformReleases(prefix string) []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	var out []string
	for _, id := range p.release {
		if len(id) >= len(prefix) && id[:len(prefix)] == prefix {
			out = append(out, id)
		}
	}
	return out
}

type listCountingOrders struct {
	*fake.InMemoryOrderRepository
	mu    sync.Mutex
	lists int
}

func (l *listCountingOrders) ListOrdersByCheckout(ctx context.Context, id string) ([]store.Order, error) {
	l.mu.Lock()
	l.lists++
	l.mu.Unlock()
	return l.InMemoryOrderRepository.ListOrdersByCheckout(ctx, id)
}

type platRig struct {
	orders *listCountingOrders
	promo  *platPromo
	svc    *order.OrderService
}

func newPlatRig(t *testing.T, promo *platPromo) *platRig {
	t.Helper()
	orders := &listCountingOrders{InMemoryOrderRepository: fake.NewInMemoryOrderRepository()}
	svc := newSvc(t, orders, fake.NewInMemoryCartRepository(), nil, nil,
		upstreamtest.NewDomain(map[string]int32{}), nil, nil,
		order.WithPromotionClient(promo),
		order.WithReleaseRetry(time.Second, 2, time.Millisecond))
	return &platRig{orders: orders, promo: promo, svc: svc}
}

func (r *platRig) place(t *testing.T, seller, checkoutID, platformCode string, due *time.Time) store.Order {
	t.Helper()
	o, err := r.orders.CreateOrder(context.Background(), store.Order{
		BuyerID: "buyer_1", SellerID: seller, TotalAmount: 3000, Currency: "VND",
		PaymentMethod: int32(paymentv1.PaymentMethod_PAYMENT_METHOD_MOCK_MOMO), PaymentDueAt: due,
		CheckoutID: checkoutID, PlatformVoucherCode: platformCode,
		Items: []store.OrderItem{{ListingID: "lst_" + seller, Quantity: 1, UnitPrice: 3000}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return o
}

func (r *platRig) setStatus(t *testing.T, id string, to store.OrderStatus) {
	t.Helper()
	if _, err := r.orders.UpdateOrderStatusFrom(context.Background(), id, to,
		[]store.OrderStatus{store.OrderStatusPending, store.OrderStatusPaid}, ""); err != nil {
		t.Fatal(err)
	}
}

func (r *platRig) status(t *testing.T, id string) store.OrderStatus {
	t.Helper()
	o, err := r.orders.GetOrder(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return o.Status
}

// Scenario: Multishop checkout: Cancelling the last unpaid order releases the platform voucher
func TestCancelLastOrderReleasesPlatformVoucher(t *testing.T) {
	r := newPlatRig(t, &platPromo{})
	a := r.place(t, "sa", "chk_1", "PLAT", nil)
	b := r.place(t, "sb", "chk_1", "PLAT", nil)
	if _, err := r.svc.CancelOrder(context.Background(), a.ID); err != nil {
		t.Fatal(err)
	}
	if got := r.promo.platformReleases("platform:"); len(got) != 0 {
		t.Fatalf("first cancel must not release the platform voucher, got %v", got)
	}
	if _, err := r.svc.CancelOrder(context.Background(), b.ID); err != nil {
		t.Fatal(err)
	}
	got := r.promo.platformReleases("platform:")
	if len(got) != 1 || got[0] != "platform:chk_1" {
		t.Fatalf("want exactly one release of platform:chk_1, got %v", got)
	}
}

// Scenario: Multishop checkout: A paid sibling blocks the platform release
func TestCancelWithPaidSiblingKeepsPlatformVoucher(t *testing.T) {
	r := newPlatRig(t, &platPromo{})
	a := r.place(t, "sa", "chk_1", "PLAT", nil)
	b := r.place(t, "sb", "chk_1", "PLAT", nil)
	r.setStatus(t, a.ID, store.OrderStatusPaid)
	if _, err := r.svc.CancelOrder(context.Background(), b.ID); err != nil {
		t.Fatal(err)
	}
	if got := r.promo.platformReleases("platform:"); len(got) != 0 {
		t.Fatalf("paid sibling must block the platform release, got %v", got)
	}
}

// Scenario: Multishop checkout: The system cancel of an expired order follows the same rule
func TestSystemCancelReleasesPlatformVoucherWhenAllCancelled(t *testing.T) {
	r := newPlatRig(t, &platPromo{})
	due := lifecycleDue
	a := r.place(t, "sa", "chk_1", "PLAT", &due)
	b := r.place(t, "sb", "chk_1", "PLAT", &due)
	if _, err := r.svc.CancelOrder(context.Background(), a.ID); err != nil {
		t.Fatal(err)
	}
	r.svc.SweepLifecycle(context.Background(), due.Add(time.Minute))
	if r.status(t, b.ID) != store.OrderStatusCancelled {
		t.Fatalf("sweeper should have cancelled the expired order")
	}
	got := r.promo.platformReleases("platform:")
	if len(got) != 1 || got[0] != "platform:chk_1" {
		t.Fatalf("want exactly one release of platform:chk_1, got %v", got)
	}
}

// Scenario: Multishop checkout: A platform release failure never fails the cancel
func TestCancelPlatformReleaseErrorNeverFailsCancel(t *testing.T) {
	r := newPlatRig(t, &platPromo{fail: true})
	a := r.place(t, "sa", "chk_1", "PLAT", nil)
	if _, err := r.svc.CancelOrder(context.Background(), a.ID); err != nil {
		t.Fatalf("cancel must succeed despite platform release failure: %v", err)
	}
	if r.status(t, a.ID) != store.OrderStatusCancelled {
		t.Fatalf("order must be Cancelled")
	}
	if len(r.promo.platformReleases("platform:")) == 0 {
		t.Fatalf("a platform release must have been attempted")
	}
}

// Scenario: Multishop checkout: Cancelling an order without a platform voucher makes no platform call
func TestCancelWithoutPlatformVoucherNoPlatformCalls(t *testing.T) {
	r := newPlatRig(t, &platPromo{})
	a := r.place(t, "sa", "", "", nil)
	b := r.place(t, "sb", "chk_2", "", nil)
	for _, id := range []string{a.ID, b.ID} {
		if _, err := r.svc.CancelOrder(context.Background(), id); err != nil {
			t.Fatal(err)
		}
	}
	if r.orders.lists != 0 {
		t.Fatalf("no ListOrdersByCheckout expected, got %d", r.orders.lists)
	}
	if got := r.promo.platformReleases("platform:"); len(got) != 0 {
		t.Fatalf("no platform release expected, got %v", got)
	}
}
