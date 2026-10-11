package handler_test

import (
	"context"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	listingv1 "github.com/buidangphuc/team-order/generated/platform/listing/v1"
	orderv1 "github.com/buidangphuc/team-order/generated/platform/order/v1"
	"github.com/buidangphuc/team-order/internal/handler"
	"github.com/buidangphuc/team-order/internal/repository"
	"github.com/buidangphuc/team-order/internal/service"
	"github.com/buidangphuc/team-order/internal/upstream/upstreamtest"
)

func checkoutHandler(t *testing.T) (*handler.OrderHandler, *upstreamtest.Domain, *repository.InMemoryOrderRepository) {
	t.Helper()
	domain := upstreamtest.NewDomain(map[string]int32{"lst_1": 10})
	orders := repository.NewInMemoryOrderRepository()
	carts := repository.NewInMemoryCartRepository()
	for _, buyer := range []string{"buyer_1", "buyer_2"} {
		if _, err := carts.AddItem(context.Background(), repository.CartItem{
			UserID: buyer, ListingID: "lst_1", Quantity: 2, UnitPrice: 1000, SellerID: "s1", Title: "item",
		}); err != nil {
			t.Fatal(err)
		}
	}
	svc := service.NewOrderService(orders, carts, nil, nil, domain, nil, nil)
	return handler.NewOrderHandler(svc, nil, nil), domain, orders
}

func withKey(ctx context.Context, keys ...string) context.Context {
	md := metadata.MD{}
	for _, k := range keys {
		md.Append("idempotency-key", k)
	}
	return metadata.NewIncomingContext(ctx, md)
}

func TestCreateOrder_InvalidIdempotencyKeyRejectedBeforeReserving(t *testing.T) {
	for name, keys := range map[string][]string{
		"too long":    {strings.Repeat("k", 256)},
		"non-ascii":   {"clé"},
		"control":     {"a\tb"},
		"blank":       {"   "},
		"two headers": {"a", "b"},
	} {
		t.Run(name, func(t *testing.T) {
			h, domain, _ := checkoutHandler(t)
			_, err := h.CreateOrder(withKey(incomingPrincipalCtx("buyer_1", "buyer"), keys...), &orderv1.CreateOrderRequest{})
			if status.Code(err) != codes.InvalidArgument {
				t.Fatalf("want InvalidArgument, got %v", err)
			}
			if domain.Calls.Reserve != 0 {
				t.Fatalf("nothing may be reserved, got %d reserve calls", domain.Calls.Reserve)
			}
		})
	}
}

func TestCreateOrder_SameKeyReturnsSameOrders(t *testing.T) {
	h, domain, orders := checkoutHandler(t)
	ctx := withKey(incomingPrincipalCtx("buyer_1", "buyer"), "K-1")
	a, err := h.CreateOrder(ctx, &orderv1.CreateOrderRequest{})
	if err != nil {
		t.Fatal(err)
	}
	b, err := h.CreateOrder(ctx, &orderv1.CreateOrderRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(a.GetOrders()) != 1 || a.GetOrders()[0].GetId() != b.GetOrders()[0].GetId() {
		t.Fatalf("want the same order ids, got %v / %v", a.GetOrders(), b.GetOrders())
	}
	if os, _ := orders.ListBuyerOrders(context.Background(), "buyer_1", 0); len(os) != 1 {
		t.Fatalf("want exactly one order, got %d", len(os))
	}
	if domain.Stock("lst_1") != 8 {
		t.Fatalf("stock %d, want 8", domain.Stock("lst_1"))
	}
	// A 255-byte key is accepted.
	if _, err := h.CreateOrder(withKey(incomingPrincipalCtx("buyer_2", "buyer"), strings.Repeat("k", 255)), &orderv1.CreateOrderRequest{}); err != nil {
		t.Fatalf("a 255-byte key must be accepted: %v", err)
	}
}

func TestCreateOrder_InProgressKeyIsAborted(t *testing.T) {
	h, domain, _ := checkoutHandler(t)
	entered := make(chan struct{})
	release := make(chan struct{})
	domain.ReserveErr = func(*listingv1.ReserveStockRequest) error {
		close(entered)
		<-release
		return nil
	}
	ctx := withKey(incomingPrincipalCtx("buyer_1", "buyer"), "K-slow")
	done := make(chan error, 1)
	go func() { _, err := h.CreateOrder(ctx, &orderv1.CreateOrderRequest{}); done <- err }()
	<-entered
	_, err := h.CreateOrder(ctx, &orderv1.CreateOrderRequest{})
	if status.Code(err) != codes.Aborted {
		t.Fatalf("want Aborted, got %v", err)
	}
	domain.ReserveErr = nil
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
