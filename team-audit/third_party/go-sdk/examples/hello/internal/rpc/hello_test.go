package rpc_test

import (
	"context"
	"io"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/buidangphuc/platform-core/packages/go-sdk/examples/hello/internal/rpc"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/auth"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/logx"
)

// A handler is a plain method, so it is tested without a server. NewTo with
// io.Discard keeps the test output quiet.
func TestGreetMapsErrors(t *testing.T) {
	h := rpc.NewHandler(logx.NewTo(io.Discard, config.Runtime{LogLevel: "error"}, "test"))

	_, err := h.Greet(context.Background(), wrapperspb.String("Ann"))
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("no principal: %v, want Unauthenticated", err)
	}

	ctx := auth.WithPrincipal(context.Background(), auth.Principal{ID: "u1", Type: "user"})
	if _, err := h.Greet(ctx, wrapperspb.String(" ")); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("empty name: %v, want InvalidArgument", err)
	}
	got, err := h.Greet(ctx, wrapperspb.String("Ann"))
	if err != nil || got.GetValue() != "hello, Ann (caller u1)" {
		t.Fatalf("Greet = %v, %v", got, err)
	}
}
