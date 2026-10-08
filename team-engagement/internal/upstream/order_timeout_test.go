package upstream

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	orderv1 "github.com/buidangphuc/team-engagement/generated/platform/order/v1"
)

// hangingOrders never answers until the caller gives up.
type hangingOrders struct {
	orderv1.UnimplementedOrderServiceServer
}

func (hangingOrders) GetOrder(ctx context.Context, _ *orderv1.GetOrderRequest) (*orderv1.GetOrderResponse, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func startHangingOrders(t *testing.T) string {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := grpc.NewServer()
	orderv1.RegisterOrderServiceServer(srv, hangingOrders{})
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	return lis.Addr().String()
}

func TestVerifyPurchase_BoundedWhenOrderHangs(t *testing.T) {
	c, err := NewOrderClient(startHangingOrders(t), 150*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()

	start := time.Now()
	// The inbound context has NO deadline: only the per-call timeout can stop it.
	_, _, err = c.VerifyPurchase(context.Background(), "buyer-1", "listing-1", "order-1")
	if err == nil {
		t.Fatal("expected an error when team-order hangs")
	}
	if el := time.Since(start); el > 2*time.Second {
		t.Fatalf("lookup was not bounded: took %v", el)
	}
}

// partiesOrders answers GetOrder from a fixed map; unknown ids are NOT_FOUND.
type partiesOrders struct {
	orderv1.UnimplementedOrderServiceServer
}

func (partiesOrders) GetOrder(_ context.Context, r *orderv1.GetOrderRequest) (*orderv1.GetOrderResponse, error) {
	switch r.GetId() {
	case "order-1":
		return &orderv1.GetOrderResponse{Order: &orderv1.Order{Id: "order-1", BuyerId: "buyer-1", SellerId: "seller-1"}}, nil
	case "order-boom":
		return nil, status.Error(codes.Internal, "db password leaked")
	}
	return nil, status.Error(codes.NotFound, "no such order")
}

func TestGetOrderParties(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := grpc.NewServer()
	orderv1.RegisterOrderServiceServer(srv, partiesOrders{})
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	c, err := NewOrderClient(lis.Addr().String(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	ctx := context.Background()

	b, s, err := c.GetOrderParties(ctx, "order-1")
	if err != nil || b != "buyer-1" || s != "seller-1" {
		t.Fatalf("got %q %q %v", b, s, err)
	}
	if _, _, err := c.GetOrderParties(ctx, "order-404"); !errors.Is(err, ErrOrderNotFound) {
		t.Fatalf("want ErrOrderNotFound, got %v", err)
	}
	if _, _, err := c.GetOrderParties(ctx, ""); !errors.Is(err, ErrOrderNotFound) {
		t.Fatalf("empty id: want ErrOrderNotFound, got %v", err)
	}
	if _, _, err := c.GetOrderParties(ctx, "order-boom"); err == nil || errors.Is(err, ErrOrderNotFound) {
		t.Fatalf("upstream failure must be a non-NotFound error, got %v", err)
	}
	var nilClient *OrderClient
	if _, _, err := nilClient.GetOrderParties(ctx, "order-1"); err == nil {
		t.Fatal("nil client must error (fail closed)")
	}
}

func TestGetOrderParties_BoundedWhenOrderHangs(t *testing.T) {
	c, err := NewOrderClient(startHangingOrders(t), 150*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	start := time.Now()
	if _, _, err = c.GetOrderParties(context.Background(), "order-1"); err == nil {
		t.Fatal("expected an error when team-order hangs")
	}
	if el := time.Since(start); el > 2*time.Second {
		t.Fatalf("lookup was not bounded: took %v", el)
	}
}
