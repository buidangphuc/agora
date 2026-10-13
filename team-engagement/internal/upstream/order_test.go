package upstream

import (
	"context"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	orderv1 "github.com/buidangphuc/team-engagement/generated/platform/order/v1"
)

// capturingOrders records the metadata of the GetOrder call it receives.
type capturingOrders struct {
	orderv1.UnimplementedOrderServiceServer
	got chan metadata.MD
}

func (c capturingOrders) GetOrder(ctx context.Context, _ *orderv1.GetOrderRequest) (*orderv1.GetOrderResponse, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	c.got <- md
	return &orderv1.GetOrderResponse{Order: &orderv1.Order{Id: "order-1", BuyerId: "b", SellerId: "s"}}, nil
}

func TestServicePrincipalInterceptor_SetsServicePrincipal(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := grpc.NewServer()
	orders := capturingOrders{got: make(chan metadata.MD, 1)}
	orderv1.RegisterOrderServiceServer(srv, orders)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	c, err := NewOrderClient(lis.Addr().String(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()

	// A stale outgoing user principal must be replaced, not forwarded.
	ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs(
		"x-principal-id", "user-1", "x-principal-type", "user", "x-principal-scopes", "admin"))
	if _, _, err := c.GetOrderParties(ctx, "order-1"); err != nil {
		t.Fatal(err)
	}
	got := <-orders.got
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
