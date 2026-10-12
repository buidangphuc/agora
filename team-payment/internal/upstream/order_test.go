package upstream_test

import (
	"context"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/auth"
	sdkupstream "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/upstream"

	orderv1 "github.com/buidangphuc/team-payment/generated/platform/order/v1"
	"github.com/buidangphuc/team-payment/internal/upstream"
)

// orderServer records the principal metadata of the GetOrder it receives.
type orderServer struct {
	orderv1.UnimplementedOrderServiceServer
	got metadata.MD
}

func (s *orderServer) GetOrder(ctx context.Context, _ *orderv1.GetOrderRequest) (*orderv1.GetOrderResponse, error) {
	s.got, _ = metadata.FromIncomingContext(ctx)
	return &orderv1.GetOrderResponse{}, nil
}

func TestServicePrincipalInterceptor_SetsServicePrincipal(t *testing.T) {
	srv := &orderServer{}
	gs := grpc.NewServer()
	orderv1.RegisterOrderServiceServer(gs, srv)
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = gs.Serve(lis) }()
	defer gs.Stop()

	conn, err := sdkupstream.Dial(lis.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()

	// A stale user principal, in the incoming request and in the outgoing metadata, must be replaced, not forwarded.
	ctx := auth.WithPrincipal(context.Background(), auth.Principal{ID: "user-1", Type: "user", Scopes: []string{"admin"}})
	ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs(
		"x-principal-id", "user-1", "x-principal-type", "user", "x-principal-scopes", "admin"))
	if _, err := upstream.NewOrderClient(conn).GetOrder(ctx, &orderv1.GetOrderRequest{Id: "o1"}); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"x-principal-id":     "service-team-payment",
		"x-principal-type":   "service",
		"x-principal-scopes": "order.read",
	}
	for k, v := range want {
		if vals := srv.got.Get(k); len(vals) != 1 || vals[0] != v {
			t.Errorf("%s = %v, want [%s]", k, vals, v)
		}
	}
}
