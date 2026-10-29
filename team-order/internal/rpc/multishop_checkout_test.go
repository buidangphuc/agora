package rpc_test

import (
	"context"
	"sync"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	identityv1 "github.com/buidangphuc/team-order/generated/platform/identity/v1"
	orderv1 "github.com/buidangphuc/team-order/generated/platform/order/v1"
	promotionv1 "github.com/buidangphuc/team-order/generated/platform/promotion/v1"
	"github.com/buidangphuc/team-order/internal/order"
	"github.com/buidangphuc/team-order/internal/rpc"
	"github.com/buidangphuc/team-order/internal/store"
	"github.com/buidangphuc/team-order/internal/store/fake"
	"github.com/buidangphuc/team-order/internal/upstream/upstreamtest"
)

type countingPromotion struct {
	promotionv1.VoucherServiceClient
	mu    sync.Mutex
	calls int
}

func (c *countingPromotion) ValidateAndReserve(_ context.Context, _ *promotionv1.ValidateAndReserveRequest, _ ...grpc.CallOption) (*promotionv1.ValidateAndReserveResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls++
	return &promotionv1.ValidateAndReserveResponse{Valid: true, DiscountAmount: 100, VoucherId: "vch_1"}, nil
}

func (c *countingPromotion) CommitReservation(_ context.Context, _ *promotionv1.CommitReservationRequest, _ ...grpc.CallOption) (*promotionv1.CommitReservationResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls++
	return &promotionv1.CommitReservationResponse{Committed: true}, nil
}

func (c *countingPromotion) ReleaseReservation(_ context.Context, _ *promotionv1.ReleaseReservationRequest, _ ...grpc.CallOption) (*promotionv1.ReleaseReservationResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls++
	return &promotionv1.ReleaseReservationResponse{Released: true}, nil
}

func (c *countingPromotion) total() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

func voucherCheckout(t *testing.T) (*rpc.OrderHandler, *countingPromotion, *fake.InMemoryOrderRepository) {
	t.Helper()
	domain := upstreamtest.NewDomain(map[string]int32{"lst_1": 10})
	orders := fake.NewInMemoryOrderRepository()
	carts := fake.NewInMemoryCartRepository()
	if _, err := carts.AddItem(context.Background(), store.CartItem{
		UserID: "buyer_1", ListingID: "lst_1", Quantity: 2, UnitPrice: 1000, SellerID: "s1", Title: "item",
	}); err != nil {
		t.Fatal(err)
	}
	promo := &countingPromotion{}
	svc := newSvc(t, orders, carts, nil, nil, domain, nil, nil,
		order.WithSagaRepository(fake.NewInMemorySagaRepository()), order.WithPromotionClient(promo))
	ac := &fakeAddrClient{addrs: []*identityv1.Address{addrA()}}
	return newHandler(t, svc, ac, nil), promo, orders
}

func assertNoOrderNoPromotion(t *testing.T, promo *countingPromotion, orders *fake.InMemoryOrderRepository) {
	t.Helper()
	if n := promo.total(); n != 0 {
		t.Fatalf("no promotion call allowed, got %d", n)
	}
	if l, _ := orders.ListBuyerOrders(context.Background(), "buyer_1", 0); len(l) != 0 {
		t.Fatalf("no order may exist: %v", l)
	}
}

func TestPreviewCheckoutEmptySelectionFailedPrecondition(t *testing.T) {
	// Scenario: Multishop checkout: Preview of an empty selection is refused
	h, _, _ := voucherCheckout(t)
	_, err := h.PreviewCheckout(incomingPrincipalCtx("buyer_1", "buyer"), &orderv1.PreviewCheckoutRequest{ItemIds: []string{"ci_nope"}})
	if status.Code(err) != codes.FailedPrecondition || status.Convert(err).Message() != "cart is empty" {
		t.Fatalf("want FailedPrecondition cart is empty, got %v", err)
	}
}

func TestPreviewCheckoutUnknownAddressFailedPrecondition(t *testing.T) {
	// Scenario: Multishop checkout: Preview with an unknown address id is refused
	h, promo, _ := voucherCheckout(t)
	_, err := h.PreviewCheckout(incomingPrincipalCtx("buyer_1", "buyer"), &orderv1.PreviewCheckoutRequest{AddressId: "addr_missing"})
	if status.Code(err) != codes.FailedPrecondition || status.Convert(err).Message() != "address not found" {
		t.Fatalf("want FailedPrecondition address not found, got %v", err)
	}
	if promo.total() != 0 {
		t.Fatalf("no promotion call allowed, got %d", promo.total())
	}
}

func TestPreviewCheckoutRequiresUserPrincipal(t *testing.T) {
	// Scenario: Multishop checkout: Preview needs a user principal
	h, _, _ := voucherCheckout(t)
	_, err := h.PreviewCheckout(serviceCtx("order.read", "order.write"), &orderv1.PreviewCheckoutRequest{})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("service principal: want PermissionDenied, got %v", err)
	}
	_, err = h.PreviewCheckout(context.Background(), &orderv1.PreviewCheckoutRequest{})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("no principal: want Unauthenticated, got %v", err)
	}
}

func TestShopVouchersDuplicateSellerInvalidArgument(t *testing.T) {
	// Scenario: Multishop checkout: Two shop vouchers for one seller are rejected
	h, promo, orders := voucherCheckout(t)
	dup := []*orderv1.ShopVoucher{{SellerId: "s1", Code: "A"}, {SellerId: "s1", Code: "B"}}
	ctx := incomingPrincipalCtx("buyer_1", "buyer")
	_, err := h.PreviewCheckout(ctx, &orderv1.PreviewCheckoutRequest{ShopVouchers: dup})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("preview: want InvalidArgument, got %v", err)
	}
	_, err = h.CreateOrder(ctx, &orderv1.CreateOrderRequest{ShopVouchers: dup})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("create: want InvalidArgument, got %v", err)
	}
	assertNoOrderNoPromotion(t, promo, orders)
}

func TestCreateOrderVouchersRequireUserPrincipal(t *testing.T) {
	// Scenario: Multishop checkout: A non-user principal cannot place a multi-voucher checkout
	h, promo, orders := voucherCheckout(t)
	_, err := h.CreateOrder(serviceCtx("order.read", "order.write"), &orderv1.CreateOrderRequest{
		ShopVouchers:        []*orderv1.ShopVoucher{{SellerId: "s1", Code: "SHOP"}},
		PlatformVoucherCode: "PLAT",
	})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("want PermissionDenied, got %v", err)
	}
	assertNoOrderNoPromotion(t, promo, orders)
}

func TestVoucherCodeNamedTwiceInvalidArgument(t *testing.T) {
	// Scenario: Multishop checkout: A voucher named twice is rejected
	h, promo, orders := voucherCheckout(t)
	ctx := incomingPrincipalCtx("buyer_1", "buyer")
	sv := []*orderv1.ShopVoucher{{SellerId: "s1", Code: "SAME"}}
	_, err := h.PreviewCheckout(ctx, &orderv1.PreviewCheckoutRequest{ShopVouchers: sv, PlatformVoucherCode: "SAME"})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("preview platform dup: want InvalidArgument, got %v", err)
	}
	_, err = h.CreateOrder(ctx, &orderv1.CreateOrderRequest{VoucherCode: "SAME", ShopVouchers: sv})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("create legacy dup: want InvalidArgument, got %v", err)
	}
	_, err = h.CreateOrder(ctx, &orderv1.CreateOrderRequest{PlatformVoucherCode: "SAME", ShopVouchers: sv})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("create platform dup: want InvalidArgument, got %v", err)
	}
	assertNoOrderNoPromotion(t, promo, orders)
}

func TestLegacyAndPlatformVoucherTogetherInvalidArgument(t *testing.T) {
	// Scenario: Multishop checkout: Legacy and platform voucher fields together are rejected
	h, promo, orders := voucherCheckout(t)
	ctx := incomingPrincipalCtx("buyer_1", "buyer")
	_, err := h.CreateOrder(ctx, &orderv1.CreateOrderRequest{VoucherCode: "OLD", PlatformVoucherCode: "PLAT"})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("create: want InvalidArgument, got %v", err)
	}
	assertNoOrderNoPromotion(t, promo, orders)
}
