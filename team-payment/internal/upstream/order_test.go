package upstream

import (
	"context"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
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
		"x-principal-id":     "service-team-payment",
		"x-principal-type":   "service",
		"x-principal-scopes": "order.read",
	}
	for k, v := range want {
		if vals := got.Get(k); len(vals) != 1 || vals[0] != v {
			t.Errorf("%s = %v, want [%s]", k, vals, v)
		}
	}
}
