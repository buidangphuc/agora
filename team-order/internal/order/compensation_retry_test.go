package order_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	listingv1 "github.com/buidangphuc/team-order/generated/platform/listing/v1"
	promotionv1 "github.com/buidangphuc/team-order/generated/platform/promotion/v1"
	"github.com/buidangphuc/team-order/internal/order"
	"github.com/buidangphuc/team-order/internal/store"
	"github.com/buidangphuc/team-order/internal/store/fake"
	"github.com/buidangphuc/team-order/internal/upstream/upstreamtest"
)

func lstAReservation(t *testing.T, r *checkoutRig) store.Reservation {
	t.Helper()
	res, err := r.sagas.ListReservationsBySaga(context.Background(), r.sagas.lastSagaID())
	if err != nil {
		t.Fatal(err)
	}
	for _, x := range res {
		if x.ListingID == "lst_a" {
			return x
		}
	}
	t.Fatalf("no lst_a reservation in %+v", res)
	return store.Reservation{}
}

type releaseCounter struct {
	mu     sync.Mutex
	counts map[string]int
}

func (c *releaseCounter) hit(id string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.counts == nil {
		c.counts = map[string]int{}
	}
	c.counts[id]++
	return c.counts[id]
}

func (c *releaseCounter) count(id string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.counts[id]
}

func failSecondSeller(t *testing.T, r *checkoutRig) {
	t.Helper()
	_, err := r.svc.CreateOrdersFromCart(context.Background(), "buyer_1", addr(), nil, 1, "")
	if !errors.Is(err, order.ErrInsufficientStock) {
		t.Fatalf("want ErrInsufficientStock, got %v", err)
	}
}

func releaseScenario(t *testing.T, fail func(attempt int) error, opts ...order.OrderServiceOption) (*checkoutRig, *releaseCounter) {
	t.Helper()
	r := twoSellerRig(t, 0, opts...)
	c := &releaseCounter{}
	r.domain.ReleaseErr = func(id string) error { return fail(c.hit(id)) }
	failSecondSeller(t, r)
	return r, c
}

func TestCompensationReleaseRetriesUnavailableOnce(t *testing.T) {
	r, c := releaseScenario(t, func(attempt int) error {
		if attempt == 1 {
			return status.Error(codes.Unavailable, "team-domain restarting")
		}
		return nil
	}, order.WithReleaseRetry(time.Second, 3, time.Millisecond))
	res := lstAReservation(t, r)
	if got := c.count(res.ID); got != 2 {
		t.Fatalf("A release succeeds on the second attempt: ReleaseStock called %d times, want 2", got)
	}
	if res.Status != store.ReservationStatusReleased {
		t.Fatalf("A release succeeds on the second attempt: reservation %v, want RELEASED", res.Status)
	}
	if got := r.domain.Stock("lst_a"); got != 10 {
		t.Fatalf("stock %d, want 10", got)
	}
}

func TestCompensationReleaseParksAfterThreeFailures(t *testing.T) {
	r, c := releaseScenario(t, func(int) error { return status.Error(codes.Unavailable, "team-domain down") })
	res := lstAReservation(t, r)
	if got := c.count(res.ID); got != 3 {
		t.Fatalf("Three failed attempts park the reservation: ReleaseStock called %d times, want 3", got)
	}
	if res.Status != store.ReservationStatusReleaseFailed {
		t.Fatalf("Three failed attempts park the reservation: reservation %v, want RELEASE_FAILED", res.Status)
	}
}

func TestCompensationReleaseNonRetryableOnce(t *testing.T) {
	r, c := releaseScenario(t, func(int) error { return status.Error(codes.FailedPrecondition, "cannot release") })
	res := lstAReservation(t, r)
	if got := c.count(res.ID); got != 1 {
		t.Fatalf("A non-retryable code is attempted once: ReleaseStock called %d times, want 1", got)
	}
	if res.Status != store.ReservationStatusReleaseFailed {
		t.Fatalf("A non-retryable code is attempted once: reservation %v, want RELEASE_FAILED", res.Status)
	}
}

func TestCompensationReleasePlainErrorIsNotRetried(t *testing.T) {
	r, c := releaseScenario(t, func(int) error { return errors.New("connection reset") })
	res := lstAReservation(t, r)
	if got := c.count(res.ID); got != 1 {
		t.Fatalf("A non-retryable code is attempted once: a non-gRPC error was attempted %d times, want 1", got)
	}
}

func TestCompensationReleaseDeadlineExceededIsRetried(t *testing.T) {
	r, c := releaseScenario(t, func(attempt int) error {
		if attempt == 1 {
			return status.Error(codes.DeadlineExceeded, "slow")
		}
		return nil
	}, order.WithReleaseRetry(time.Second, 3, time.Millisecond))
	res := lstAReservation(t, r)
	if got := c.count(res.ID); got != 2 {
		t.Fatalf("A release succeeds on the second attempt: DeadlineExceeded must be retried, got %d calls", got)
	}
}

func TestWithReleaseRetryBoundsAttempts(t *testing.T) {
	r, c := releaseScenario(t, func(int) error { return status.Error(codes.Unavailable, "team-domain down") },
		order.WithReleaseRetry(time.Second, 2, time.Millisecond))
	res := lstAReservation(t, r)
	if got := c.count(res.ID); got != 2 {
		t.Fatalf("WithReleaseRetry still bounds the attempts: ReleaseStock called %d times, want 2", got)
	}
	if res.Status != store.ReservationStatusReleaseFailed {
		t.Fatalf("WithReleaseRetry still bounds the attempts: reservation %v, want RELEASE_FAILED", res.Status)
	}
}

func TestCancelInlineReleaseBudgetParksSlowRelease(t *testing.T) {
	r, o := newCancelRigWithRetry(t, "", 30*time.Second)
	r.domain.ReleaseHang = true
	start := time.Now()
	res, err := r.svc.CancelOrder(context.Background(), o.ID)
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("A buyer cancel stays within the inline release budget: took %v, want under 3s", elapsed)
	}
	if err != nil || res.Status != store.OrderStatusCancelled {
		t.Fatalf("A buyer cancel stays within the inline release budget: %v %+v", err, res)
	}
	got, err := r.orders.GetOrder(context.Background(), o.ID)
	if err != nil || got.Status != store.OrderStatusCancelled {
		t.Fatalf("order must read CANCELLED: %v %+v", err, got)
	}
	held, err := r.sagas.ListReservationsByOrder(context.Background(), o.ID)
	if err != nil || len(held) != 1 || held[0].Status != store.ReservationStatusReleaseFailed {
		t.Fatalf("reservation must be RELEASE_FAILED for the sweep: %v %+v", err, held)
	}
}

type recordedLogs struct{ buf bytes.Buffer }

func (l *recordedLogs) logger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(&l.buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func (l *recordedLogs) warnOrAbove() []map[string]any {
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(l.buf.String()), "\n") {
		if line == "" {
			continue
		}
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			continue
		}
		if lvl, _ := rec["level"].(string); lvl == "WARN" || lvl == "ERROR" {
			out = append(out, rec)
		}
	}
	return out
}

func TestCancelFlashAndVoucherReleaseFailureIsLoggedOnly(t *testing.T) {
	logs := &recordedLogs{}
	domain := &pricedDomain{
		Domain:   upstreamtest.NewDomain(map[string]int32{"lst_1": 10}),
		listings: map[string]*listingv1.Listing{"lst_1": {Id: "lst_1", Price: 500000, SellerId: "s1", Stock: 10}},
	}
	orders := fake.NewInMemoryOrderRepository()
	promo := newFakePromotion(500)
	flash := &holdFake{
		sales: map[string]*promotionv1.GetActiveFlashSaleResponse{"lst_1": activeSale("", 350000)},
		releaseFn: func(*promotionv1.ReleaseFlashSaleStockRequest) error {
			return status.Error(codes.Unavailable, "promotion down")
		},
	}
	carts := fake.NewInMemoryCartRepository()
	if _, err := carts.AddItem(context.Background(), store.CartItem{
		UserID: "buyer_1", ListingID: "lst_1",
		Quantity: 2, UnitPrice: 350000, SellerID: "s1", Title: "lst_1",
	}); err != nil {
		t.Fatal(err)
	}
	svc := newSvc(t, orders, carts, nil, nil, domain, nil, logs.logger(),
		order.WithSagaRepository(fake.NewInMemorySagaRepository()), order.WithPromotionClient(promo),
		order.WithOrderFlashSaleClient(flash), order.WithReleaseRetry(time.Second, 2, time.Millisecond))
	placed, err := svc.CreateOrdersFromCart(context.Background(), "buyer_1", addr(), nil, 1, "VOUCHER")
	if err != nil || len(placed) != 1 {
		t.Fatalf("checkout: %v", err)
	}
	promo.releaseErr = status.Error(codes.Unavailable, "promotion down")
	logs.buf.Reset()

	res, err := svc.CancelOrder(context.Background(), placed[0].ID)
	if err != nil || res.Status != store.OrderStatusCancelled {
		t.Fatalf("A failed flash or voucher release never fails the cancel: %v %+v", err, res)
	}
	got, _ := orders.GetOrder(context.Background(), placed[0].ID)
	if got.Status != store.OrderStatusCancelled {
		t.Fatalf("order must read CANCELLED, got %v", got.Status)
	}
	if n := len(flash.releasedIDs()); n != 1 {
		t.Fatalf("flash release is best effort and never retried: %d calls", n)
	}
	if n := len(promo.releasedIDs); n != 1 {
		t.Fatalf("voucher release is best effort and never retried: %d calls", n)
	}
	if recs := logs.warnOrAbove(); len(recs) != 2 {
		t.Fatalf("each failure is logged once, want 2 warn/error records, got %d: %v", len(recs), recs)
	}
}

func TestNormalizeIdempotencyKeyRejectsNonASCII(t *testing.T) {
	for _, key := range []string{"café", "k-é", "é", "ключ"} {
		k, err := order.NormalizeIdempotencyKey(key)
		if !errors.Is(err, order.ErrInvalidIdempotencyKey) {
			t.Errorf("A non-ASCII key is rejected: %q -> %q, %v; want ErrInvalidIdempotencyKey", key, k, err)
		}
	}
	for _, key := range []string{"k-1", "  k-1  ", "A~z 09"} {
		if _, err := order.NormalizeIdempotencyKey(key); err != nil {
			t.Errorf("printable ASCII %q must be accepted: %v", key, err)
		}
	}
	if k, err := order.NormalizeIdempotencyKey("  k-1  "); err != nil || k != "k-1" {
		t.Errorf("a padded key must be trimmed: %q %v", k, err)
	}
}

func TestIdempotencyKeyIsScopedToBuyer(t *testing.T) {
	r := newKeyedRig(t)
	r.addToCart(t, "buyer_1")
	r.addToCart(t, "buyer_2")
	a, err := r.checkout("buyer_1", "k-1")
	if err != nil {
		t.Fatal(err)
	}
	b, err := r.checkout("buyer_2", "k-1")
	if err != nil {
		t.Fatal(err)
	}
	for _, ob := range b {
		for _, oa := range a {
			if ob.ID == oa.ID {
				t.Fatalf("Another buyer's key does not replay their checkout: buyer B received %s", ob.ID)
			}
		}
		if ob.BuyerID != "buyer_2" {
			t.Fatalf("buyer B's order belongs to %q", ob.BuyerID)
		}
	}
	if len(b) == 0 || r.orderCount(t, "buyer_2") != len(b) {
		t.Fatalf("buyer B must own their new orders: %d", r.orderCount(t, "buyer_2"))
	}
}
