package order_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	listingv1 "github.com/buidangphuc/team-order/generated/platform/listing/v1"
	"github.com/buidangphuc/team-order/internal/order"
	"github.com/buidangphuc/team-order/internal/store"
	"github.com/buidangphuc/team-order/internal/store/fake"
	"github.com/buidangphuc/team-order/internal/upstream/upstreamtest"
)

type keyedRig struct {
	domain *upstreamtest.Domain
	orders *fake.InMemoryOrderRepository
	sagas  *fake.InMemorySagaRepository
	carts  *fake.InMemoryCartRepository
	svc    *order.OrderService
}

// newKeyedRig: listing lst_1 (seller s1) with stock 10; addToCart puts 2 of it in
// a buyer's cart.
func newKeyedRig(t *testing.T) *keyedRig {
	r := &keyedRig{
		domain: upstreamtest.NewDomain(map[string]int32{"lst_1": 10}),
		orders: fake.NewInMemoryOrderRepository(),
		sagas:  fake.NewInMemorySagaRepository(),
		carts:  fake.NewInMemoryCartRepository(),
	}
	r.svc = newSvc(t, r.orders, r.carts, nil, nil, r.domain, nil, nil, order.WithSagaRepository(r.sagas))
	return r
}

func (r *keyedRig) addToCart(t *testing.T, buyer string) {
	t.Helper()
	if _, err := r.carts.AddItem(context.Background(), store.CartItem{
		UserID: buyer, ListingID: "lst_1", Quantity: 2, UnitPrice: 1000, SellerID: "s1", Title: "item",
	}); err != nil {
		t.Fatal(err)
	}
}

func (r *keyedRig) checkout(buyer, key string) ([]store.Order, error) {
	return r.svc.CreateOrdersFromCart(context.Background(), buyer, addr(), nil, 1, "", order.WithIdempotencyKey(key))
}

func (r *keyedRig) orderCount(t *testing.T, buyer string) int {
	t.Helper()
	os, err := r.orders.ListBuyerOrders(context.Background(), buyer, 0)
	if err != nil {
		t.Fatal(err)
	}
	return len(os)
}

func ids(os []store.Order) []string {
	var out []string
	for _, o := range os {
		out = append(out, o.ID)
	}
	return out
}

func TestNormalizeIdempotencyKey(t *testing.T) {
	long := make([]byte, 256)
	for i := range long {
		long[i] = 'a'
	}
	for _, bad := range []string{"", "   ", string(long), "café", "a\x01b"} {
		if _, err := order.NormalizeIdempotencyKey(bad); !errors.Is(err, order.ErrInvalidIdempotencyKey) {
			t.Errorf("%q: want ErrInvalidIdempotencyKey, got %v", bad, err)
		}
	}
	if k, err := order.NormalizeIdempotencyKey("  " + string(long[:255]) + " "); err != nil || len(k) != 255 {
		t.Fatalf("a 255-byte key must be accepted (trimmed): %v %d", err, len(k))
	}
}

func TestKeyedCheckout_SameKeyReturnsSameOrders(t *testing.T) {
	r := newKeyedRig(t)
	r.addToCart(t, "buyer_1")
	first, err := r.checkout("buyer_1", "K1")
	if err != nil {
		t.Fatal(err)
	}
	second, err := r.checkout("buyer_1", "K1")
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 1 || len(second) != 1 || first[0].ID != second[0].ID {
		t.Fatalf("replay must return the same orders: %v vs %v", ids(first), ids(second))
	}
	if n := r.orderCount(t, "buyer_1"); n != 1 {
		t.Fatalf("want exactly one order, got %d", n)
	}
	if s := r.domain.Stock("lst_1"); s != 8 {
		t.Fatalf("stock %d, want 8", s)
	}
	if r.domain.Calls.Reserve != 1 {
		t.Fatalf("a replay must not reserve again: %d reserve calls", r.domain.Calls.Reserve)
	}
}

func TestKeyedCheckout_ConcurrentDuplicatesCreateOneOrderSet(t *testing.T) {
	r := newKeyedRig(t)
	r.addToCart(t, "buyer_1")
	// Slow the reserve down so duplicates overlap the first attempt.
	r.domain.ReserveErr = func(*listingv1.ReserveStockRequest) error {
		time.Sleep(20 * time.Millisecond)
		return nil
	}
	const n = 8
	var wg sync.WaitGroup
	start := make(chan struct{})
	results := make([][]store.Order, n)
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			results[i], errs[i] = r.checkout("buyer_1", "K-concurrent")
		}(i)
	}
	close(start)
	wg.Wait()

	var orderID string
	succeeded := 0
	for i := 0; i < n; i++ {
		if errs[i] != nil {
			if !errors.Is(errs[i], order.ErrCheckoutInProgress) {
				t.Fatalf("call %d: want orders or ErrCheckoutInProgress, got %v", i, errs[i])
			}
			continue
		}
		succeeded++
		if len(results[i]) != 1 || (orderID != "" && results[i][0].ID != orderID) {
			t.Fatalf("call %d returned a different order set: %v", i, ids(results[i]))
		}
		orderID = results[i][0].ID
	}
	if succeeded == 0 {
		t.Fatal("at least one call must return the orders")
	}
	if c := r.orderCount(t, "buyer_1"); c != 1 {
		t.Fatalf("want exactly one order, got %d", c)
	}
	if s := r.domain.Stock("lst_1"); s != 8 {
		t.Fatalf("stock must be reduced once, got %d", s)
	}
}

func TestKeyedCheckout_InProgressIsAborted(t *testing.T) {
	r := newKeyedRig(t)
	r.addToCart(t, "buyer_1")
	entered := make(chan struct{})
	release := make(chan struct{})
	r.domain.ReserveErr = func(*listingv1.ReserveStockRequest) error {
		close(entered)
		<-release
		return nil
	}
	done := make(chan error, 1)
	go func() { _, err := r.checkout("buyer_1", "K-slow"); done <- err }()
	<-entered
	if _, err := r.checkout("buyer_1", "K-slow"); !errors.Is(err, order.ErrCheckoutInProgress) {
		t.Fatalf("a duplicate while the first attempt runs must be ErrCheckoutInProgress, got %v", err)
	}
	r.domain.ReserveErr = nil
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestKeyedCheckout_FailedCheckoutFreesTheKey(t *testing.T) {
	r := newKeyedRig(t)
	r.domain.SetStock("lst_1", 1) // the cart wants 2
	r.addToCart(t, "buyer_1")
	if _, err := r.checkout("buyer_1", "K-free"); !errors.Is(err, order.ErrInsufficientStock) {
		t.Fatalf("want ErrInsufficientStock, got %v", err)
	}
	r.domain.SetStock("lst_1", 10) // restock
	got, err := r.checkout("buyer_1", "K-free")
	if err != nil || len(got) != 1 {
		t.Fatalf("the key of a failed checkout must be reusable: %v %d", err, len(got))
	}
	if s := r.domain.Stock("lst_1"); s != 8 {
		t.Fatalf("stock %d, want 8", s)
	}
}

func TestKeyedCheckout_EmptyCartFreesTheKey(t *testing.T) {
	r := newKeyedRig(t)
	if _, err := r.checkout("buyer_1", "K-empty"); !errors.Is(err, order.ErrEmptyCart) {
		t.Fatalf("want ErrEmptyCart, got %v", err)
	}
	r.addToCart(t, "buyer_1")
	if got, err := r.checkout("buyer_1", "K-empty"); err != nil || len(got) != 1 {
		t.Fatalf("an abandoned key must be reusable: %v", err)
	}
}

func TestKeyedCheckout_KeysAreScopedToTheBuyer(t *testing.T) {
	r := newKeyedRig(t)
	r.addToCart(t, "buyer_1")
	r.addToCart(t, "buyer_2")
	a, err := r.checkout("buyer_1", "SAME")
	if err != nil {
		t.Fatal(err)
	}
	b, err := r.checkout("buyer_2", "SAME")
	if err != nil {
		t.Fatal(err)
	}
	if a[0].ID == b[0].ID || a[0].BuyerID != "buyer_1" || b[0].BuyerID != "buyer_2" {
		t.Fatalf("each buyer must get their own order: %+v / %+v", a[0], b[0])
	}
	if s := r.domain.Stock("lst_1"); s != 6 {
		t.Fatalf("stock %d, want 6", s)
	}
}

func TestUnkeyedCheckout_RunsFreshEveryTime(t *testing.T) {
	r := newKeyedRig(t)
	r.addToCart(t, "buyer_1")
	if _, err := r.svc.CreateOrdersFromCart(context.Background(), "buyer_1", addr(), nil, 1, ""); err != nil {
		t.Fatal(err)
	}
	r.addToCart(t, "buyer_1")
	if _, err := r.svc.CreateOrdersFromCart(context.Background(), "buyer_1", addr(), nil, 1, ""); err != nil {
		t.Fatal(err)
	}
	if n := r.orderCount(t, "buyer_1"); n != 2 {
		t.Fatalf("two unkeyed checkouts must create two orders, got %d", n)
	}
}

// failOnceCart fails the first RemoveItems, as a lost cart clear after placement.
type failOnceCart struct {
	*fake.InMemoryCartRepository
	mu     sync.Mutex
	failed bool
}

func (c *failOnceCart) RemoveItems(ctx context.Context, userID string, ids []string) error {
	c.mu.Lock()
	first := !c.failed
	c.failed = true
	c.mu.Unlock()
	if first {
		return errors.New("cart store down")
	}
	return c.InMemoryCartRepository.RemoveItems(ctx, userID, ids)
}

func TestKeyedReplay_RetriesTheCartClear(t *testing.T) {
	r := newKeyedRig(t)
	cart := &failOnceCart{InMemoryCartRepository: r.carts}
	r.svc = newSvc(t, r.orders, cart, nil, nil, r.domain, nil, nil, order.WithSagaRepository(r.sagas))
	r.addToCart(t, "buyer_1")
	if _, err := r.checkout("buyer_1", "K-cart"); err != nil {
		t.Fatal(err)
	}
	if items, _ := r.carts.GetCart(context.Background(), "buyer_1"); len(items) != 1 {
		t.Fatalf("first clear was made to fail; cart should still hold the item, got %d", len(items))
	}
	if _, err := r.checkout("buyer_1", "K-cart"); err != nil {
		t.Fatal(err)
	}
	if items, _ := r.carts.GetCart(context.Background(), "buyer_1"); len(items) != 0 {
		t.Fatalf("the replay must clear the checked-out item, cart has %d", len(items))
	}
}
