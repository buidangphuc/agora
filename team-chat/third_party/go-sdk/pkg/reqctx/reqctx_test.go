package reqctx_test

import (
	"context"
	"testing"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/reqctx"
)

func TestRequestIDRoundTrip(t *testing.T) {
	ctx := reqctx.WithRequestID(context.Background(), "req-1")
	if got := reqctx.RequestID(ctx); got != "req-1" {
		t.Fatalf("RequestID = %q, want req-1", got)
	}
}

func TestPrincipalIDRoundTrip(t *testing.T) {
	ctx := reqctx.WithPrincipalID(context.Background(), "user-7")
	if got := reqctx.PrincipalID(ctx); got != "user-7" {
		t.Fatalf("PrincipalID = %q, want user-7", got)
	}
}

func TestEmptyWhenAbsent(t *testing.T) {
	ctx := context.Background()
	if got := reqctx.RequestID(ctx); got != "" {
		t.Fatalf("RequestID = %q, want empty", got)
	}
	if got := reqctx.PrincipalID(ctx); got != "" {
		t.Fatalf("PrincipalID = %q, want empty", got)
	}
	// the two keys must not collide
	ctx = reqctx.WithRequestID(ctx, "r")
	if got := reqctx.PrincipalID(ctx); got != "" {
		t.Fatalf("PrincipalID = %q after setting only request id", got)
	}
}
