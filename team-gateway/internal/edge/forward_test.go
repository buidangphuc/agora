package edge_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"

	orderv1 "github.com/buidangphuc/team-gateway/generated/platform/order/v1"

	searchv1 "github.com/buidangphuc/team-gateway/generated/platform/search/v1"
	"github.com/buidangphuc/team-gateway/generated/platform/search/v1/searchv1connect"
	"github.com/buidangphuc/team-gateway/internal/edge"
)

const idempotencyRefusal = "invalid Idempotency-Key header: must be a single printable-ASCII value of at most 255 characters"

// Scenario: The anonymous principal is forwarded with the public scopes.
func TestAnonymousPrincipalIsForwardedWithPublicScopes(t *testing.T) {
	spy := &upstreamSpy{}
	e := edge.NewEdge(nil, []string{"listing.read", "search:read"}, time.Second, 0, 1000, 1000)
	opts := connect.WithInterceptors(e.Interceptors(slog.New(slog.NewTextHandler(io.Discard, nil)))...)
	mux := http.NewServeMux()
	p, h := searchv1connect.NewSearchServiceHandler(edge.NewSearchForwarder(spySearch{spy: spy}, e), opts)
	mux.Handle(p, h)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := searchv1connect.NewSearchServiceClient(srv.Client(), srv.URL)
	req := connect.NewRequest(&searchv1.SearchListingsRequest{})
	if _, err := c.SearchListings(context.Background(), req); err != nil {
		t.Fatalf("call: %v", err)
	}
	if got := spy.md.Get("x-principal-id"); len(got) != 1 || got[0] != "anonymous" {
		t.Errorf("x-principal-id = %v, want [anonymous]", got)
	}
	if got := spy.md.Get("x-principal-type"); len(got) != 1 || got[0] != "anonymous" {
		t.Errorf("x-principal-type = %v, want [anonymous]", got)
	}
	if got := spy.md.Get("x-principal-scopes"); len(got) != 1 || got[0] != "listing.read,search:read" {
		t.Errorf("x-principal-scopes = %v, want [listing.read,search:read]", got)
	}
}

// Scenario: An invalid idempotency key is refused (256 chars, control character, empty, blank).
func TestValidIdempotencyKey(t *testing.T) {
	client, up := newHeaderFixture(t)
	for name, key := range map[string]string{
		"256 chars":    strings.Repeat("k", 256),
		"control char": "a\tb",
		"empty":        "",
		"blank":        "   ",
		"non-ascii":    "khóa",
	} {
		up.calls, up.md = 0, nil
		req := connect.NewRequest(&orderv1.CreateOrderRequest{})
		req.Header().Set("Idempotency-Key", key)
		_, err := client.CreateOrder(context.Background(), req)
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("%s: code = %v, want invalid_argument (%v)", name, connect.CodeOf(err), err)
			continue
		}
		if msg := err.(*connect.Error).Message(); msg != idempotencyRefusal {
			t.Errorf("%s: message = %q, want %q", name, msg, idempotencyRefusal)
		}
		if up.calls != 0 {
			t.Errorf("%s: invalid key reached upstream", name)
		}
	}
	for _, key := range []string{strings.Repeat("k", 255), "with space and ~ symbols!?"} {
		up.calls, up.md = 0, nil
		req := connect.NewRequest(&orderv1.CreateOrderRequest{})
		req.Header().Set("Idempotency-Key", key)
		if _, err := client.CreateOrder(context.Background(), req); err != nil {
			t.Errorf("valid key %q refused: %v", key, err)
		}
	}
}

// Scenario: A repeated idempotency key is refused (and surrounding whitespace is trimmed).
func TestRepeatedIdempotencyKeyIsRefusedAndWhitespaceTrimmed(t *testing.T) {
	client, up := newHeaderFixture(t)

	req := connect.NewRequest(&orderv1.CreateOrderRequest{})
	req.Header().Add("Idempotency-Key", "ord-123")
	req.Header().Add("Idempotency-Key", "ord-456")
	_, err := client.CreateOrder(context.Background(), req)
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("repeated header: code = %v, want invalid_argument (%v)", connect.CodeOf(err), err)
	}
	if msg := err.(*connect.Error).Message(); msg != idempotencyRefusal {
		t.Errorf("message = %q, want %q", msg, idempotencyRefusal)
	}
	if up.calls != 0 {
		t.Fatalf("repeated key reached upstream %d times", up.calls)
	}

	req = connect.NewRequest(&orderv1.CreateOrderRequest{})
	req.Header().Set("Idempotency-Key", "  ord-123  ")
	if _, err := client.CreateOrder(context.Background(), req); err != nil {
		t.Fatalf("padded key refused: %v", err)
	}
	if got := up.md.Get("idempotency-key"); len(got) != 1 || got[0] != "ord-123" {
		t.Fatalf("idempotency-key metadata = %v, want [ord-123]", got)
	}
}
