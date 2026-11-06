package upstream

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	orderv1 "github.com/buidangphuc/team-promotion/generated/platform/order/v1"
)

type hasCompletedOrders struct {
	orderv1.UnimplementedOrderServiceServer
	mu     sync.Mutex
	mds    []metadata.MD
	buyers []string
	has    bool
	err    error
	block  bool
}

func (h *hasCompletedOrders) HasCompletedOrder(ctx context.Context, req *orderv1.HasCompletedOrderRequest) (*orderv1.HasCompletedOrderResponse, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	h.mu.Lock()
	h.mds = append(h.mds, md)
	h.buyers = append(h.buyers, req.GetBuyerId())
	h.mu.Unlock()
	if h.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if h.err != nil {
		return nil, h.err
	}
	return &orderv1.HasCompletedOrderResponse{HasCompleted: h.has}, nil
}

func startOrders(t *testing.T, svc *hasCompletedOrders) string {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := grpc.NewServer()
	orderv1.RegisterOrderServiceServer(srv, svc)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	return lis.Addr().String()
}

func newOrderClient(t *testing.T, addr string, timeout time.Duration) *OrderClient {
	t.Helper()
	c, err := NewOrderClient(addr, timeout)
	if err != nil || c == nil {
		t.Fatalf("NewOrderClient: %v, %v", c, err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func TestHasCompletedOrderAnswers(t *testing.T) {
	for _, want := range []bool{true, false} {
		svc := &hasCompletedOrders{has: want}
		c := newOrderClient(t, startOrders(t, svc), time.Second)
		got, err := c.HasCompletedOrder(t.Context(), "buyer-1")
		if err != nil || got != want {
			t.Fatalf("HasCompletedOrder = %v, %v, want %v nil", got, err, want)
		}
		if len(svc.buyers) != 1 || svc.buyers[0] != "buyer-1" {
			t.Fatalf("upstream got buyers %v, want [buyer-1]", svc.buyers)
		}
	}
}

func TestOrderCallCarriesServicePrincipal(t *testing.T) {
	svc := &hasCompletedOrders{}
	c := newOrderClient(t, startOrders(t, svc), time.Second)
	caller := metadata.NewOutgoingContext(t.Context(), metadata.Pairs(
		"x-principal-id", "buyer-1", "x-principal-type", "user", "x-principal-scopes", "admin"))
	if _, err := c.HasCompletedOrder(caller, "buyer-1"); err != nil {
		t.Fatal(err)
	}
	if len(svc.mds) != 1 {
		t.Fatalf("upstream calls = %d, want 1", len(svc.mds))
	}
	md := svc.mds[0]
	want := map[string]string{
		"x-principal-id":     "service-team-promotion",
		"x-principal-type":   "service",
		"x-principal-scopes": "order.read",
	}
	for k, v := range want {
		if got := md.Get(k); len(got) != 1 || got[0] != v {
			t.Errorf("%s = %v, want [%s]", k, got, v)
		}
	}
}

func TestOrderClientFailsClosed(t *testing.T) {
	for name, serverErr := range map[string]error{
		"internal":         status.Error(codes.Internal, "boom"),
		"unavailable":      status.Error(codes.Unavailable, "down"),
		"permission":       status.Error(codes.PermissionDenied, "missing order.read"),
		"unauthenticated":  status.Error(codes.Unauthenticated, "no principal"),
		"invalid argument": status.Error(codes.InvalidArgument, "buyer_id"),
	} {
		t.Run(name, func(t *testing.T) {
			c := newOrderClient(t, startOrders(t, &hasCompletedOrders{err: serverErr, has: true}), time.Second)
			got, err := c.HasCompletedOrder(t.Context(), "buyer-1")
			if status.Code(err) != codes.Unavailable {
				t.Fatalf("err = %v, want gRPC Unavailable", err)
			}
			if got {
				t.Fatal("a failed check must not answer true")
			}
		})
	}
}

func TestOrderClientTimeoutFailsClosed(t *testing.T) {
	c := newOrderClient(t, startOrders(t, &hasCompletedOrders{block: true}), 50*time.Millisecond)
	start := time.Now()
	got, err := c.HasCompletedOrder(t.Context(), "buyer-1")
	if status.Code(err) != codes.Unavailable || got {
		t.Fatalf("got %v, %v, want false and Unavailable", got, err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("call took %v, the client timeout was not applied", elapsed)
	}
}

func TestOrderClientUnreachableFailsClosed(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := lis.Addr().String()
	_ = lis.Close()
	c := newOrderClient(t, addr, 200*time.Millisecond)
	if got, err := c.HasCompletedOrder(t.Context(), "buyer-1"); status.Code(err) != codes.Unavailable || got {
		t.Fatalf("got %v, %v, want false and Unavailable", got, err)
	}
}

func TestUnconfiguredOrderClientFailsClosed(t *testing.T) {
	var c *OrderClient
	if got, err := c.HasCompletedOrder(t.Context(), "buyer-1"); status.Code(err) != codes.Unavailable || got {
		t.Fatalf("nil client: got %v, %v, want false and Unavailable", got, err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("Close on nil client: %v", err)
	}
}
