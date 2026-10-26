package upstream

import (
	"context"
	"net"
	"sync"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	orderv1 "github.com/buidangphuc/team-payment/generated/platform/order/v1"
)

type recordingOrder struct {
	orderv1.UnimplementedOrderServiceServer
	mu  sync.Mutex
	mds []metadata.MD
}

func (r *recordingOrder) GetOrder(ctx context.Context, _ *orderv1.GetOrderRequest) (*orderv1.GetOrderResponse, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	r.mu.Lock()
	r.mds = append(r.mds, md)
	r.mu.Unlock()
	return &orderv1.GetOrderResponse{}, nil
}

func startOrder(t *testing.T, svc *recordingOrder) string {
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

// Scenario: The audit call is made as the service principal (order client half: a stale user principal is replaced by the service principal)
func TestOrderClientUsesServicePrincipal(t *testing.T) {
	rec := &recordingOrder{}
	client, conn, err := DialOrderService(startOrder(t, rec))
	if err != nil || client == nil || conn == nil {
		t.Fatalf("DialOrderService: %v, %v, %v", client, conn, err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs(
		"x-principal-id", "user-1", "x-principal-type", "user", "x-principal-scopes", "admin"))
	if _, err := client.GetOrder(ctx, &orderv1.GetOrderRequest{}); err != nil {
		t.Fatalf("GetOrder: %v", err)
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.mds) != 1 {
		t.Fatalf("want 1 call, got %d", len(rec.mds))
	}
	want := map[string]string{
		"x-principal-id":     "service-team-payment",
		"x-principal-type":   "service",
		"x-principal-scopes": "order.read",
	}
	for k, v := range want {
		if vals := rec.mds[0].Get(k); len(vals) != 1 || vals[0] != v {
			t.Errorf("%s = %v, want [%s]", k, vals, v)
		}
	}
}
