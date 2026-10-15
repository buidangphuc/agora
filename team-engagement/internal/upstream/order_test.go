package upstream

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	orderv1 "github.com/buidangphuc/team-engagement/generated/platform/order/v1"
)

func TestServicePrincipalInterceptor_SetsServicePrincipal(t *testing.T) {
	// A stale outgoing user principal must be replaced, not forwarded.
	ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs(
		"x-principal-id", "user-1", "x-principal-type", "user", "x-principal-scopes", "admin"))
	var got metadata.MD
	invoker := func(ctx context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		got, _ = metadata.FromOutgoingContext(ctx)
		return nil
	}
	if err := servicePrincipalInterceptor(ctx, "/platform.order.v1.OrderService/GetOrder", nil, nil, nil, invoker); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"x-principal-id":     "service-team-engagement",
		"x-principal-type":   "service",
		"x-principal-scopes": "order.read",
	}
	for k, v := range want {
		if vals := got.Get(k); len(vals) != 1 || vals[0] != v {
			t.Errorf("%s = %v, want [%s]", k, vals, v)
		}
	}
}

// verifyOrders serves canned orders for VerifyPurchase classification.
type verifyOrders struct {
	orderv1.UnimplementedOrderServiceServer
}

func (verifyOrders) GetOrder(_ context.Context, r *orderv1.GetOrderRequest) (*orderv1.GetOrderResponse, error) {
	item := []*orderv1.OrderItem{{ListingId: "listing-1"}}
	switch r.GetId() {
	case "order-done":
		return &orderv1.GetOrderResponse{Order: &orderv1.Order{Id: "order-done", BuyerId: "buyer-1", SellerId: "seller-1",
			Status: orderv1.OrderStatus_ORDER_STATUS_COMPLETED, Items: item}}, nil
	case "order-pending":
		return &orderv1.GetOrderResponse{Order: &orderv1.Order{Id: "order-pending", BuyerId: "buyer-1", SellerId: "seller-1",
			Status: orderv1.OrderStatus_ORDER_STATUS_PENDING, Items: item}}, nil
	case "order-boom":
		return nil, status.Error(codes.Internal, "boom")
	case "order-down":
		return nil, status.Error(codes.Unavailable, "down")
	}
	return nil, status.Error(codes.NotFound, "no such order")
}

func newVerifyClient(t *testing.T) *OrderClient {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := grpc.NewServer()
	orderv1.RegisterOrderServiceServer(srv, verifyOrders{})
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	c, err := NewOrderClient(lis.Addr().String(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

// Scenario: Upstream not-found is not an outage.
func TestVerifyPurchaseNotFoundIsIneligible(t *testing.T) {
	ok, _, err := newVerifyClient(t).VerifyPurchase(context.Background(), "buyer-1", "listing-1", "order-404")
	if err != nil || ok {
		t.Fatalf("NotFound must be ineligible (false, nil); got ok=%v err=%v", ok, err)
	}
}

// Scenario: Upstream not-found is not an outage (ineligible classes are not errors).
func TestVerifyPurchaseIneligibleClasses_NotFoundIsNotAnOutage(t *testing.T) {
	c := newVerifyClient(t)
	ctx := context.Background()
	cases := []struct {
		name, buyer, listing, order string
	}{
		{"not completed", "buyer-1", "listing-1", "order-pending"},
		{"other buyer", "buyer-2", "listing-1", "order-done"},
		{"listing not in order", "buyer-1", "listing-9", "order-done"},
	}
	for _, tc := range cases {
		ok, _, err := c.VerifyPurchase(ctx, tc.buyer, tc.listing, tc.order)
		if err != nil || ok {
			t.Errorf("%s: want (false, nil), got ok=%v err=%v", tc.name, ok, err)
		}
	}
	ok, seller, err := c.VerifyPurchase(ctx, "buyer-1", "listing-1", "order-done")
	if err != nil || !ok || seller != "seller-1" {
		t.Errorf("eligible: got ok=%v seller=%q err=%v", ok, seller, err)
	}
}

// Scenario: Verifier returns an error (client half): non-NotFound upstream failures are errors.
func TestVerifyPurchaseUpstreamFailureIsError(t *testing.T) {
	c := newVerifyClient(t)
	for _, id := range []string{"order-boom", "order-down"} {
		ok, _, err := c.VerifyPurchase(context.Background(), "buyer-1", "listing-1", id)
		if err == nil || ok {
			t.Errorf("%s: want error, got ok=%v err=%v", id, ok, err)
		}
	}
}

// Scenario: No order upstream configured (nil client must not look like "ineligible").
func TestVerifyPurchaseNilClientIsNotIneligible(t *testing.T) {
	var c *OrderClient
	ok, _, err := c.VerifyPurchase(context.Background(), "buyer-1", "listing-1", "order-done")
	if ok || !errors.Is(err, ErrOrderClientNotConfigured) {
		t.Fatalf("nil client: want ErrOrderClientNotConfigured, got ok=%v err=%v", ok, err)
	}
}

// Scenario: Review without an order is refused (empty order id is not a silent false,nil).
func TestVerifyPurchaseEmptyOrderIDIsError(t *testing.T) {
	ok, _, err := newVerifyClient(t).VerifyPurchase(context.Background(), "buyer-1", "listing-1", "")
	if ok || !errors.Is(err, ErrOrderIDRequired) {
		t.Fatalf("empty order id: want ErrOrderIDRequired, got ok=%v err=%v", ok, err)
	}
}
