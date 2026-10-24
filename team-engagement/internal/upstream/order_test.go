package upstream

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	orderv1 "github.com/buidangphuc/team-engagement/generated/platform/order/v1"
)

// verifyOrders serves canned orders for VerifyPurchase classification.
type verifyOrders struct {
	orderv1.UnimplementedOrderServiceServer
}

func (verifyOrders) GetOrder(_ context.Context, r *orderv1.GetOrderRequest) (*orderv1.GetOrderResponse, error) {
	item := []*orderv1.OrderItem{{ListingId: "listing-1"}}
	switch r.GetId() {
	case "order-done":
		return &orderv1.GetOrderResponse{Order: &orderv1.Order{
			Id: "order-done", BuyerId: "buyer-1", SellerId: "seller-1",
			Status: orderv1.OrderStatus_ORDER_STATUS_COMPLETED, Items: item,
		}}, nil
	case "order-pending":
		return &orderv1.GetOrderResponse{Order: &orderv1.Order{
			Id: "order-pending", BuyerId: "buyer-1", SellerId: "seller-1",
			Status: orderv1.OrderStatus_ORDER_STATUS_PENDING, Items: item,
		}}, nil
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
	c, err := NewOrderClient(lis.Addr().String())
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

// Scenario: Review without an order is refused (empty order id is not a silent false,nil).
func TestVerifyPurchaseEmptyOrderIDIsError(t *testing.T) {
	ok, _, err := newVerifyClient(t).VerifyPurchase(context.Background(), "buyer-1", "listing-1", "")
	if ok || !errors.Is(err, ErrOrderIDRequired) {
		t.Fatalf("empty order id: want ErrOrderIDRequired, got ok=%v err=%v", ok, err)
	}
}

type returnOrders struct {
	orderv1.UnimplementedOrderServiceServer
	mu  sync.Mutex
	md  metadata.MD
	ids []string
}

func (r *returnOrders) GetReturnRequest(ctx context.Context, req *orderv1.GetReturnRequestRequest) (*orderv1.GetReturnRequestResponse, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	r.mu.Lock()
	r.md = md
	r.ids = append(r.ids, req.GetId())
	r.mu.Unlock()
	switch req.GetId() {
	case "ret-rejected":
		return &orderv1.GetReturnRequestResponse{ReturnRequest: &orderv1.OrderReturn{
			Id: "ret-rejected", OrderId: "order-1",
			Status: orderv1.ReturnStatus_RETURN_STATUS_REJECTED, CanEscalate: true,
		}}, nil
	case "ret-open":
		return &orderv1.GetReturnRequestResponse{ReturnRequest: &orderv1.OrderReturn{
			Id: "ret-open", OrderId: "order-2",
			Status: orderv1.ReturnStatus_RETURN_STATUS_PENDING, CanEscalate: false,
		}}, nil
	case "ret-boom":
		return nil, status.Error(codes.Internal, "db password leaked")
	case "ret-down":
		return nil, status.Error(codes.Unavailable, "down")
	case "ret-denied":
		return nil, status.Error(codes.PermissionDenied, "no")
	case "ret-hang":
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return nil, status.Error(codes.NotFound, "no such return")
}

func newReturnClient(t *testing.T) (*OrderClient, *returnOrders) {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	fake := &returnOrders{}
	srv := grpc.NewServer()
	orderv1.RegisterOrderServiceServer(srv, fake)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	c, err := NewOrderClient(lis.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c, fake
}

// Scenario: Dispute escalation: Buyer escalates a rejected return
func TestGetReturn_DisputeEscalation_ReturnsOrderStatusAndCanEscalate(t *testing.T) {
	c, _ := newReturnClient(t)
	orderID, st, can, err := c.GetReturn(context.Background(), "ret-rejected")
	if err != nil || orderID != "order-1" || st != orderv1.ReturnStatus_RETURN_STATUS_REJECTED || !can {
		t.Fatalf("got order=%q status=%v can=%v err=%v", orderID, st, can, err)
	}
	orderID, st, can, err = c.GetReturn(context.Background(), "ret-open")
	if err != nil || orderID != "order-2" || st != orderv1.ReturnStatus_RETURN_STATUS_PENDING || can {
		t.Fatalf("not escalatable: got order=%q status=%v can=%v err=%v", orderID, st, can, err)
	}
}

// Scenario: Dispute escalation: Buyer escalates a rejected return (service principal with order.read)
func TestGetReturn_DisputeEscalation_UsesServicePrincipalNotCaller(t *testing.T) {
	c, fake := newReturnClient(t)
	ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs(
		"x-principal-id", "user-1", "x-principal-type", "user", "x-principal-scopes", "admin"))
	if _, _, _, err := c.GetReturn(ctx, "ret-rejected"); err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	want := map[string]string{
		"x-principal-id":     "service-team-engagement",
		"x-principal-type":   "service",
		"x-principal-scopes": "order.read",
	}
	for k, v := range want {
		if vals := fake.md.Get(k); len(vals) != 1 || vals[0] != v {
			t.Errorf("%s = %v, want [%s]", k, vals, v)
		}
	}
}

// Scenario: Dispute escalation: Buyer escalates a rejected return (unknown return maps to ErrReturnNotFound)
func TestGetReturn_DisputeEscalation_NotFoundIsErrReturnNotFound(t *testing.T) {
	c, _ := newReturnClient(t)
	if _, _, _, err := c.GetReturn(context.Background(), "ret-404"); !errors.Is(err, ErrReturnNotFound) {
		t.Fatalf("want ErrReturnNotFound, got %v", err)
	}
	if _, _, _, err := c.GetReturn(context.Background(), ""); !errors.Is(err, ErrReturnNotFound) {
		t.Fatalf("empty id: want ErrReturnNotFound, got %v", err)
	}
}

// Scenario: Dispute escalation: Team-order unavailable fails closed
func TestGetReturn_DisputeEscalation_UpstreamFailureFailsClosed(t *testing.T) {
	c, _ := newReturnClient(t)
	for _, id := range []string{"ret-boom", "ret-down", "ret-denied"} {
		orderID, _, can, err := c.GetReturn(context.Background(), id)
		if err == nil {
			t.Fatalf("%s: want error", id)
		}
		if errors.Is(err, ErrReturnNotFound) || errors.Is(err, ErrOrderClientNotConfigured) {
			t.Errorf("%s: outage misclassified: %v", id, err)
		}
		if orderID != "" || can {
			t.Errorf("%s: failure must not carry data: order=%q can=%v", id, orderID, can)
		}
		if strings.Contains(err.Error(), "password") {
			t.Errorf("%s: upstream detail leaked: %v", id, err)
		}
	}
}
