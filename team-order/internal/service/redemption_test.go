package service_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"google.golang.org/grpc"

	promotionv1 "github.com/buidangphuc/team-order/generated/platform/promotion/v1"
	"github.com/buidangphuc/team-order/internal/repository"
	"github.com/buidangphuc/team-order/internal/service"
)

// fakePromotion models team-promotion's VoucherService. ValidateAndReserve is
// idempotent on reservation_id (mirrors the contract): each unique id is one quota
// decrement. It records the ids it committed/released so a test can assert the
// hold was resolved exactly once.
type fakePromotion struct {
	mu sync.Mutex

	reservedIDs  map[string]int // reservation_id -> reserve count
	committedIDs []string
	releasedIDs  []string

	discount   int64
	valid      bool
	reason     string
	reserveErr error
}

func newFakePromotion(discount int64) *fakePromotion {
	return &fakePromotion{reservedIDs: map[string]int{}, discount: discount, valid: true}
}

func (f *fakePromotion) ValidateAndReserve(_ context.Context, in *promotionv1.ValidateAndReserveRequest, _ ...grpc.CallOption) (*promotionv1.ValidateAndReserveResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.reserveErr != nil {
		return nil, f.reserveErr
	}
	f.reservedIDs[in.GetReservationId()]++
	if !f.valid {
		return &promotionv1.ValidateAndReserveResponse{Valid: false, Reason: f.reason}, nil
	}
	return &promotionv1.ValidateAndReserveResponse{Valid: true, DiscountAmount: f.discount, VoucherId: "vch_1"}, nil
}

func (f *fakePromotion) CommitReservation(_ context.Context, in *promotionv1.CommitReservationRequest, _ ...grpc.CallOption) (*promotionv1.CommitReservationResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.committedIDs = append(f.committedIDs, in.GetReservationId())
	return &promotionv1.CommitReservationResponse{Committed: true}, nil
}

func (f *fakePromotion) ReleaseReservation(_ context.Context, in *promotionv1.ReleaseReservationRequest, _ ...grpc.CallOption) (*promotionv1.ReleaseReservationResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.releasedIDs = append(f.releasedIDs, in.GetReservationId())
	return &promotionv1.ReleaseReservationResponse{Released: true}, nil
}

func (f *fakePromotion) reserveCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.reservedIDs)
}

func (f *fakePromotion) reservedHas(id string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.reservedIDs[id]
	return ok
}

// A valid voucher records the discount on the order and reduces its total.
func TestCreateOrders_ValidVoucher_DiscountAndReducedTotal(t *testing.T) {
	ctx := context.Background()
	domain := newFakeDomain()
	orderRepo := newFakeOrderRepo()
	saga := repository.NewInMemorySagaRepository()
	cart := &fakeCartRepo{items: []repository.CartItem{{ID: "ci_1", ListingID: "lst_1", Quantity: 1, SellerID: "s1", UnitPrice: 100000}}}
	promo := newFakePromotion(30000)

	svc := service.NewOrderService(orderRepo, cart, nil, nil, domain, nil, nil,
		service.WithSagaRepository(saga), service.WithPromotionClient(promo))

	orders, err := svc.CreateOrdersFromCart(ctx, "buyer_1", addr(), nil, 1, "SAVE30")
	if err != nil {
		t.Fatalf("checkout with valid voucher failed: %v", err)
	}
	if len(orders) != 1 {
		t.Fatalf("expected 1 order, got %d", len(orders))
	}
	o := orders[0]
	if o.DiscountAmount != 30000 {
		t.Fatalf("expected discount_amount 30000, got %d", o.DiscountAmount)
	}
	if o.VoucherCode != "SAVE30" {
		t.Fatalf("expected voucher_code recorded on order, got %q", o.VoucherCode)
	}
	// items 100000 + shipping 20000 (Hà Nội, < 500k) - discount 30000 = 90000
	if o.TotalAmount != 90000 {
		t.Fatalf("expected total 90000, got %d", o.TotalAmount)
	}
	if promo.reserveCount() != 1 {
		t.Fatalf("expected exactly 1 voucher reserve, got %d", promo.reserveCount())
	}
}

// Without a voucher the total is unchanged and the promotion service is never
// called — the pre-voucher path is byte-for-byte the same.
func TestCreateOrders_NoVoucher_Unchanged(t *testing.T) {
	ctx := context.Background()
	domain := newFakeDomain()
	orderRepo := newFakeOrderRepo()
	saga := repository.NewInMemorySagaRepository()
	cart := &fakeCartRepo{items: []repository.CartItem{{ID: "ci_1", ListingID: "lst_1", Quantity: 1, SellerID: "s1", UnitPrice: 100000}}}
	promo := newFakePromotion(30000)

	svc := service.NewOrderService(orderRepo, cart, nil, nil, domain, nil, nil,
		service.WithSagaRepository(saga), service.WithPromotionClient(promo))

	orders, err := svc.CreateOrdersFromCart(ctx, "buyer_1", addr(), nil, 1, "")
	if err != nil {
		t.Fatalf("checkout without voucher failed: %v", err)
	}
	o := orders[0]
	if o.DiscountAmount != 0 || o.VoucherCode != "" {
		t.Fatalf("expected no discount/voucher, got discount=%d voucher=%q", o.DiscountAmount, o.VoucherCode)
	}
	if o.TotalAmount != 120000 { // 100000 + 20000 shipping, no discount
		t.Fatalf("expected total 120000, got %d", o.TotalAmount)
	}
	if promo.reserveCount() != 0 {
		t.Fatalf("promotion must not be called without a voucher, got %d reserves", promo.reserveCount())
	}
}

// An invalid/expired voucher rejects the whole checkout with ErrVoucherRejected
// (carrying the promotion reason) and releases the stock reserved so far — the
// voucher is never silently dropped.
func TestCreateOrders_InvalidVoucher_RejectedAndStockReleased(t *testing.T) {
	ctx := context.Background()
	domain := newFakeDomain()
	orderRepo := newFakeOrderRepo()
	saga := repository.NewInMemorySagaRepository()
	cart := &fakeCartRepo{items: []repository.CartItem{{ID: "ci_1", ListingID: "lst_1", Quantity: 1, SellerID: "s1", UnitPrice: 100000}}}
	promo := newFakePromotion(0)
	promo.valid = false
	promo.reason = "voucher expired"

	svc := service.NewOrderService(orderRepo, cart, nil, nil, domain, nil, nil,
		service.WithSagaRepository(saga), service.WithPromotionClient(promo))

	_, err := svc.CreateOrdersFromCart(ctx, "buyer_1", addr(), nil, 1, "EXPIRED")
	if err == nil {
		t.Fatal("expected checkout to fail for an invalid voucher")
	}
	if !errors.Is(err, service.ErrVoucherRejected) {
		t.Fatalf("expected ErrVoucherRejected, got %v", err)
	}
	if !strings.Contains(err.Error(), "voucher expired") {
		t.Fatalf("expected promotion reason in error, got %q", err.Error())
	}
	if orderRepo.count() != 0 {
		t.Fatalf("no order should be created for a rejected voucher, got %d", orderRepo.count())
	}
	if !domain.releaseSeen {
		t.Fatal("stock reserved before voucher validation must be released on rejection")
	}
	if !domain.releasedListing("lst_1") {
		t.Fatal("expected the reserved listing to be released")
	}
}

// When the saga fails after a voucher hold is placed (order persist fails), the
// hold is released on compensation — exactly once, for the id it was reserved under.
func TestCreateOrders_CompensationReleasesVoucher(t *testing.T) {
	ctx := context.Background()
	domain := newFakeDomain()
	orderRepo := newFakeOrderRepo()
	orderRepo.failFor = "s1" // reserve + voucher succeed, then persist fails → compensate
	saga := repository.NewInMemorySagaRepository()
	cart := &fakeCartRepo{items: []repository.CartItem{{ID: "ci_1", ListingID: "lst_1", Quantity: 1, SellerID: "s1", UnitPrice: 100000}}}
	promo := newFakePromotion(30000)

	svc := service.NewOrderService(orderRepo, cart, nil, nil, domain, nil, nil,
		service.WithSagaRepository(saga), service.WithPromotionClient(promo))

	_, err := svc.CreateOrdersFromCart(ctx, "buyer_1", addr(), nil, 1, "SAVE30")
	if err == nil {
		t.Fatal("expected checkout to fail at persist")
	}
	if len(promo.releasedIDs) != 1 {
		t.Fatalf("expected exactly 1 voucher release on compensation, got %d", len(promo.releasedIDs))
	}
	if !promo.reservedHas(promo.releasedIDs[0]) {
		t.Fatalf("released id %q was never reserved", promo.releasedIDs[0])
	}
	if len(promo.committedIDs) != 0 {
		t.Fatalf("no voucher should be committed on a failed saga, got %d", len(promo.committedIDs))
	}
}

// A single-seller checkout with several items validates/reserves the voucher
// exactly once (quota is not decremented per item) and applies one discount.
func TestCreateOrders_SingleSeller_ValidatesVoucherOnce(t *testing.T) {
	ctx := context.Background()
	domain := newFakeDomain()
	orderRepo := newFakeOrderRepo()
	saga := repository.NewInMemorySagaRepository()
	cart := &fakeCartRepo{items: []repository.CartItem{
		{ID: "ci_1", ListingID: "lst_1", Quantity: 1, SellerID: "s1", UnitPrice: 100000},
		{ID: "ci_2", ListingID: "lst_2", Quantity: 2, SellerID: "s1", UnitPrice: 50000},
	}}
	promo := newFakePromotion(30000)

	svc := service.NewOrderService(orderRepo, cart, nil, nil, domain, nil, nil,
		service.WithSagaRepository(saga), service.WithPromotionClient(promo))

	orders, err := svc.CreateOrdersFromCart(ctx, "buyer_1", addr(), nil, 1, "SAVE30")
	if err != nil {
		t.Fatalf("checkout failed: %v", err)
	}
	if len(orders) != 1 {
		t.Fatalf("expected 1 seller-order, got %d", len(orders))
	}
	if promo.reserveCount() != 1 {
		t.Fatalf("expected exactly 1 voucher reserve across items, got %d", promo.reserveCount())
	}
	if orders[0].DiscountAmount != 30000 {
		t.Fatalf("expected discount applied once (30000), got %d", orders[0].DiscountAmount)
	}
}
