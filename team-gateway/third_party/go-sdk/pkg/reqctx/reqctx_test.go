package reqctx_test

import (
	"context"
	"strings"
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

func TestValidRequestID(t *testing.T) {
	for _, id := range []string{"a", "req-1", "A.b_c-9", strings.Repeat("x", 128)} {
		if !reqctx.ValidRequestID(id) {
			t.Errorf("ValidRequestID(%q) = false", id)
		}
	}
	for _, id := range []string{"", "has space", "new\nline", "ünï", "a/b", strings.Repeat("x", 129)} {
		if reqctx.ValidRequestID(id) {
			t.Errorf("ValidRequestID(%q) = true", id)
		}
	}
}
