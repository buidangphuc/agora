package upstream

import (
	"context"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"

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
