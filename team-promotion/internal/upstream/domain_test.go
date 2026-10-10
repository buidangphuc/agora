package upstream

import (
	"context"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

func TestForwardPrincipalKeepsCallerAndAddsListingRead(t *testing.T) {
	in := metadata.NewIncomingContext(context.Background(), metadata.Pairs(
		"x-principal-id", "seller-1",
		"x-principal-type", "user",
		"x-principal-scopes", "listing.write",
	))
	var got metadata.MD
	invoker := func(ctx context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		got, _ = metadata.FromOutgoingContext(ctx)
		return nil
	}
	if err := forwardPrincipalInterceptor()(in, "/m", nil, nil, nil, invoker); err != nil {
		t.Fatal(err)
	}
	if v := got.Get("x-principal-id"); len(v) != 1 || v[0] != "seller-1" {
		t.Fatalf("principal id = %v, want caller", v)
	}
	if v := got.Get("x-principal-scopes"); len(v) != 1 || !hasScope(v[0], "listing.read") || !hasScope(v[0], "listing.write") {
		t.Fatalf("scopes = %v", v)
	}
}
