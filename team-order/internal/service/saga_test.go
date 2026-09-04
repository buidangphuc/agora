package service_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"

	listingv1 "github.com/buidangphuc/team-order/generated/platform/listing/v1"
	"github.com/buidangphuc/team-order/internal/repository"
	"github.com/buidangphuc/team-order/internal/service"
)

// ── test doubles ──

// fakeDomainClient models team-domain's stock service. ReserveStock is idempotent
// on reservation_id (mirrors the W1-T2 contract): repeated reserves with the same
// id decrement only once. It records the context each ReleaseStock ran under so a
// test can assert compensation used a live (background) context.
type fakeDomainClient struct {
	mu sync.Mutex

	reservedIDs  map[string]int // reservation_id -> effective decrement count
	failReserve  map[string]bool
	releasedIDs  []string
	releasedLIDs []string
	releaseCtxOK bool // true if every ReleaseStock ran under a non-cancelled ctx
	releaseSeen  bool
	releaseErr   error
}

func newFakeDomain() *fakeDomainClient {
	return &fakeDomainClient{reservedIDs: map[string]int{}, failReserve: map[string]bool{}, releaseCtxOK: true}
}

func (f *fakeDomainClient) GetListing(_ context.Context, _ *listingv1.GetListingRequest, _ ...grpc.CallOption) (*listingv1.GetListingResponse, error) {
	return &listingv1.GetListingResponse{}, nil
}

func (f *fakeDomainClient) ReserveStock(_ context.Context, req *listingv1.ReserveStockRequest, _ ...grpc.CallOption) (*listingv1.ReserveStockResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failReserve[req.GetListingId()] {
		return &listingv1.ReserveStockResponse{Success: false, Message: "out of stock"}, errors.New("insufficient stock")
	}
	f.reservedIDs[req.GetReservationId()]++ // idempotent: unique ids == effective decrements
	return &listingv1.ReserveStockResponse{Success: true}, nil
}

func (f *fakeDomainClient) ReleaseStock(ctx context.Context, req *listingv1.ReleaseStockRequest, _ ...grpc.CallOption) (*listingv1.ReleaseStockResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.releaseSeen = true
	if ctx.Err() != nil {
		f.releaseCtxOK = false
	}
	if f.releaseErr != nil {
		return nil, f.releaseErr
	}
	f.releasedIDs = append(f.releasedIDs, req.GetReservationId())
	f.releasedLIDs = append(f.releasedLIDs, req.GetListingId())
	return &listingv1.ReleaseStockResponse{Success: true}, nil
}

func (f *fakeDomainClient) uniqueDecrements() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.reservedIDs)
}

func (f *fakeDomainClient) releasedListing(listingID string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, l := range f.releasedLIDs {
		if l == listingID {
			return true
		}
	}
	return false
}

// fakeOrderRepo is an in-memory order repo whose CreateOrder can be made to fail.
type fakeOrderRepo struct {
	mu       sync.Mutex
	orders   map[string]repository.Order
	calls    int
	failCall int    // fail the Nth CreateOrder call (0 = never)
	failFor  string // fail CreateOrder when SellerID == failFor ("" = ignore)
}

func newFakeOrderRepo() *fakeOrderRepo {
	return &fakeOrderRepo{orders: map[string]repository.Order{}}
}

func (r *fakeOrderRepo) CreateOrder(_ context.Context, o repository.Order) (repository.Order, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	if (r.failCall != 0 && r.calls == r.failCall) || (r.failFor != "" && o.SellerID == r.failFor) {
		return repository.Order{}, errors.New("simulated persist failure")
	}
	if o.ID == "" {
		o.ID = "ord_" + o.SellerID + "_" + time.Now().Format("150405.000000000")
	}
	o.Status = repository.OrderStatusPending
	r.orders[o.ID] = o
	return o, nil
}

func (r *fakeOrderRepo) GetOrder(_ context.Context, id string) (repository.Order, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	o, ok := r.orders[id]
	if !ok {
		return repository.Order{}, repository.ErrOrderNotFound
	}
	return o, nil
}

func (r *fakeOrderRepo) ListBuyerOrders(_ context.Context, _ string, _ int32) ([]repository.Order, error) {
	return nil, nil
}
func (r *fakeOrderRepo) ListSellerOrders(_ context.Context, _ string, _ int32) ([]repository.Order, error) {
	return nil, nil
}
func (r *fakeOrderRepo) UpdateOrderStatus(_ context.Context, id string, st repository.OrderStatus, _ string) (repository.Order, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	o, ok := r.orders[id]
	if !ok {
		return repository.Order{}, repository.ErrOrderNotFound
	}
	o.Status = st
	r.orders[id] = o
	return o, nil
}

func (r *fakeOrderRepo) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.orders)
}

// fakeCartRepo lets a test seed items and optionally fail RemoveItems.
type fakeCartRepo struct {
	items         []repository.CartItem
	removeErr     error
	removeCalled  bool
	removedItemID []string
}

func (c *fakeCartRepo) GetCart(_ context.Context, _ string) ([]repository.CartItem, error) {
	return c.items, nil
}
func (c *fakeCartRepo) AddItem(_ context.Context, _ repository.CartItem) ([]repository.CartItem, error) {
	return c.items, nil
}
func (c *fakeCartRepo) UpdateItem(_ context.Context, _, _ string, _ int32) ([]repository.CartItem, error) {
	return c.items, nil
}
func (c *fakeCartRepo) RemoveItem(_ context.Context, _, _ string) ([]repository.CartItem, error) {
	return c.items, nil
}
func (c *fakeCartRepo) ClearCart(_ context.Context, _ string) error { return nil }
func (c *fakeCartRepo) RemoveItems(_ context.Context, _ string, ids []string) error {
	c.removeCalled = true
	c.removedItemID = ids
	return c.removeErr
}

func addr() repository.Address { return repository.Address{City: "Hà Nội"} }

// ── AD5/M6: the caller sets a stable reservation_id per (cart_item, attempt) ──

func TestReservationID_StableAndDistinct(t *testing.T) {
	item := repository.CartItem{ID: "ci_1", ListingID: "lst_1", VariantID: "", Quantity: 2}
	a := service.ReservationID("buyer_1", item)
	b := service.ReservationID("buyer_1", item)
	if a == "" {
		t.Fatal("reservation id must not be empty")
	}
	if a != b {
		t.Fatalf("reservation id must be stable across retries: %s != %s", a, b)
	}
	other := service.ReservationID("buyer_1", repository.CartItem{ID: "ci_2", ListingID: "lst_1", Quantity: 2})
	if a == other {
		t.Fatal("different cart items must get different reservation ids")
	}
}

// A retried checkout with the same cart item reuses the same reservation_id, so
// team-domain's idempotent reserve decrements stock only once (SA-M6).
func TestCreateOrders_RetryUsesSameReservationID_SingleDecrement(t *testing.T) {
	ctx := context.Background()
	domain := newFakeDomain()
	// Both attempts fail at persistence so the cart item survives for the retry.
	orderRepo := newFakeOrderRepo()
	orderRepo.failFor = "s1" // every attempt fails at persistence, so the cart item survives
	saga := repository.NewInMemorySagaRepository()
	cart := &fakeCartRepo{items: []repository.CartItem{{ID: "ci_1", ListingID: "lst_1", Quantity: 1, SellerID: "s1", UnitPrice: 1000}}}

	newSvc := func() *service.OrderService {
		return service.NewOrderService(orderRepo, cart, nil, nil, domain, nil, nil, service.WithSagaRepository(saga))
	}

	// Attempt 1 (fails at persist) then attempt 2 (fails again). Same reservation_id.
	if _, err := newSvc().CreateOrdersFromCart(ctx, "buyer_1", addr(), nil, 1, ""); err == nil {
		t.Fatal("expected persist failure on attempt 1")
	}
	if _, err := newSvc().CreateOrdersFromCart(ctx, "buyer_1", addr(), nil, 1, ""); err == nil {
		t.Fatal("expected persist failure on attempt 2")
	}

	if got := domain.uniqueDecrements(); got != 1 {
		t.Fatalf("expected exactly 1 effective stock decrement across retries, got %d", got)
	}
}

// ── SA-C2: crash between ReserveStock and persist → TTL sweep releases stock ──

func TestSweepExpiredReservations_ReleasesLeakedStock(t *testing.T) {
	ctx := context.Background()
	domain := newFakeDomain()
	orderRepo := newFakeOrderRepo()
	saga := repository.NewInMemorySagaRepository()

	// Simulate a crashed checkout: stock was reserved but the process died before
	// the order persisted, leaving a RESERVED reservation whose TTL has elapsed.
	_, _ = saga.CreateReservation(ctx, repository.Reservation{
		ID:        "res_leaked",
		SagaID:    "saga_dead",
		ListingID: "lst_1",
		Quantity:  3,
		Status:    repository.ReservationStatusReserved,
		ExpiresAt: time.Now().Add(-time.Minute),
	})

	svc := service.NewOrderService(orderRepo, &fakeCartRepo{}, nil, nil, domain, nil, nil, service.WithSagaRepository(saga))

	n, err := svc.SweepExpiredReservations(ctx, time.Now())
	if err != nil {
		t.Fatalf("sweep failed: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 reservation released by sweep, got %d", n)
	}
	if !domain.releasedListing("lst_1") {
		t.Fatal("expected ReleaseStock called for the leaked listing")
	}
	got, _ := saga.GetReservation(ctx, "res_leaked")
	if got.Status != repository.ReservationStatusReleased {
		t.Fatalf("expected reservation RELEASED, got status %d", got.Status)
	}
	if orderRepo.count() != 0 {
		t.Fatal("no orphan order should exist after sweep")
	}
}

// ── AD3: compensation runs on a background ctx even when the request ctx is dead ──

func TestCompensation_RunsOnBackgroundCtx_WhenRequestCancelled(t *testing.T) {
	domain := newFakeDomain()
	orderRepo := newFakeOrderRepo()
	orderRepo.failFor = "s1" // force compensation after a successful reserve
	saga := repository.NewInMemorySagaRepository()
	cart := &fakeCartRepo{items: []repository.CartItem{{ID: "ci_1", ListingID: "lst_1", Quantity: 1, SellerID: "s1", UnitPrice: 1000}}}

	svc := service.NewOrderService(orderRepo, cart, nil, nil, domain, nil, nil, service.WithSagaRepository(saga))

	// The request context is already cancelled when checkout runs.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := svc.CreateOrdersFromCart(ctx, "buyer_1", addr(), nil, 1, "")
	if err == nil {
		t.Fatal("expected checkout to fail")
	}
	if !domain.releaseSeen {
		t.Fatal("expected compensation to release reserved stock")
	}
	if !domain.releaseCtxOK {
		t.Fatal("compensation must run on a live background ctx, not the cancelled request ctx")
	}
	if !domain.releasedListing("lst_1") {
		t.Fatal("expected the reserved listing to be released")
	}
}

// ── SA-M7: multi-seller partial failure never releases a persisted order's stock ──

func TestMultiSeller_PartialFailure_DoesNotReleaseCommittedStock(t *testing.T) {
	ctx := context.Background()
	domain := newFakeDomain()
	orderRepo := newFakeOrderRepo()
	orderRepo.failCall = 2 // first seller-order persists; the second fails
	saga := repository.NewInMemorySagaRepository()
	cart := &fakeCartRepo{items: []repository.CartItem{
		{ID: "ci_a", ListingID: "lst_a", Quantity: 1, SellerID: "sa", UnitPrice: 1000},
		{ID: "ci_b", ListingID: "lst_b", Quantity: 1, SellerID: "sb", UnitPrice: 2000},
	}}

	svc := service.NewOrderService(orderRepo, cart, nil, nil, domain, nil, nil, service.WithSagaRepository(saga))

	_, err := svc.CreateOrdersFromCart(ctx, "buyer_1", addr(), nil, 1, "")
	if err == nil {
		t.Fatal("expected checkout to fail because the second seller-order failed to persist")
	}

	// Exactly one order persisted (the first seller's).
	if orderRepo.count() != 1 {
		t.Fatalf("expected 1 persisted order, got %d", orderRepo.count())
	}
	// Exactly one reservation released (the failed seller's un-committed one).
	if got := len(domain.releasedLIDs); got != 1 {
		t.Fatalf("expected exactly 1 release, got %d (%v)", got, domain.releasedLIDs)
	}

	// The persisted order's stock must NOT have been released (M7).
	var persisted repository.Order
	for _, o := range orderRepo.orders {
		persisted = o
	}
	persistedListing := persisted.Items[0].ListingID
	if domain.releasedListing(persistedListing) {
		t.Fatalf("persisted order's stock (%s) was wrongly released", persistedListing)
	}

	// And its reservation is COMMITTED, not releasable.
	resList, _ := saga.ListReservationsBySaga(ctx, findSagaID(t, saga))
	var committed, released int
	for _, r := range resList {
		switch r.Status {
		case repository.ReservationStatusCommitted:
			committed++
		case repository.ReservationStatusReleased:
			released++
		}
	}
	if committed != 1 || released != 1 {
		t.Fatalf("expected 1 committed + 1 released reservation, got committed=%d released=%d", committed, released)
	}
}

// findSagaID returns the saga id shared by the checkout's reservations, looked up
// via their deterministic reservation ids (same id inputs as the service uses).
func findSagaID(t *testing.T, saga *repository.InMemorySagaRepository) string {
	t.Helper()
	for _, item := range []repository.CartItem{
		{ID: "ci_a", ListingID: "lst_a", Quantity: 1},
		{ID: "ci_b", ListingID: "lst_b", Quantity: 1},
	} {
		if r, err := saga.GetReservation(context.Background(), service.ReservationID("buyer_1", item)); err == nil {
			return r.SagaID
		}
	}
	t.Fatal("could not locate saga id")
	return ""
}
