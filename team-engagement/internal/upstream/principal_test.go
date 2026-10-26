package upstream

import (
	"context"
	"net"
	"strings"
	"sync"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	orderv1 "github.com/buidangphuc/team-engagement/generated/platform/order/v1"
)

type recordingOrders struct {
	orderv1.UnimplementedOrderServiceServer
	mu     sync.Mutex
	orders []metadata.MD
	rets   []metadata.MD
}

func (r *recordingOrders) GetOrder(ctx context.Context, _ *orderv1.GetOrderRequest) (*orderv1.GetOrderResponse, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	r.mu.Lock()
	r.orders = append(r.orders, md)
	r.mu.Unlock()
	return &orderv1.GetOrderResponse{Order: &orderv1.Order{Id: "order-1", BuyerId: "buyer-1", SellerId: "seller-1"}}, nil
}

func (r *recordingOrders) GetReturnRequest(ctx context.Context, _ *orderv1.GetReturnRequestRequest) (*orderv1.GetReturnRequestResponse, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	r.mu.Lock()
	r.rets = append(r.rets, md)
	r.mu.Unlock()
	return &orderv1.GetReturnRequestResponse{ReturnRequest: &orderv1.OrderReturn{
		Id: "ret-1", OrderId: "order-1", Status: orderv1.ReturnStatus_RETURN_STATUS_REJECTED, CanEscalate: true,
	}}, nil
}

func newRecordingClient(t *testing.T) (*OrderClient, *recordingOrders) {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	rec := &recordingOrders{}
	srv := grpc.NewServer()
	orderv1.RegisterOrderServiceServer(srv, rec)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	c, err := NewOrderClient(lis.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c, rec
}

func callerCtx() context.Context {
	return metadata.NewOutgoingContext(context.Background(), metadata.Pairs(
		"x-principal-id", "buyer-1", "x-principal-type", "user", "x-principal-scopes", "admin"))
}

func assertOrderPrincipal(t *testing.T, label string, md metadata.MD) {
	t.Helper()
	want := map[string]string{
		"x-principal-id":     "service-team-engagement",
		"x-principal-type":   "service",
		"x-principal-scopes": "order.read",
	}
	for k, v := range want {
		if vals := md.Get(k); len(vals) != 1 || vals[0] != v {
			t.Errorf("%s: %s = %v, want [%s]", label, k, vals, v)
		}
	}
	for k, vals := range md {
		if !strings.HasPrefix(k, "x-principal-") {
			continue
		}
		for _, v := range vals {
			if strings.Contains(v, "buyer-1") || v == "user" || v == "admin" {
				t.Errorf("%s: caller principal leaked in %s=%v", label, k, vals)
			}
		}
	}
}

// Scenario: Engagement SDK: Order call carries the order.read service principal
func TestOrderCallCarriesServicePrincipal(t *testing.T) {
	c, rec := newRecordingClient(t)
	if _, _, err := c.GetOrderParties(callerCtx(), "order-1"); err != nil {
		t.Fatal(err)
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.orders) != 1 {
		t.Fatalf("want 1 GetOrder call, got %d", len(rec.orders))
	}
	assertOrderPrincipal(t, "GetOrder", rec.orders[0])
}

// Scenario: Engagement SDK: Return lookup carries the order.read service principal
func TestReturnCallCarriesServicePrincipal(t *testing.T) {
	c, rec := newRecordingClient(t)
	ctx := callerCtx()
	orderID, _, _, err := c.GetReturn(ctx, "ret-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := c.GetOrderParties(ctx, orderID); err != nil {
		t.Fatal(err)
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.rets) != 1 || len(rec.orders) != 1 {
		t.Fatalf("want 1 GetReturn and 1 GetOrder call, got %d and %d", len(rec.rets), len(rec.orders))
	}
	assertOrderPrincipal(t, "GetReturn", rec.rets[0])
	assertOrderPrincipal(t, "GetOrder", rec.orders[0])
}
