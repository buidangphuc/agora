package edge_test

import (
	"context"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	orderv1 "github.com/buidangphuc/team-gateway/generated/platform/order/v1"
	"github.com/buidangphuc/team-gateway/generated/platform/order/v1/orderv1connect"
	"github.com/buidangphuc/team-gateway/internal/edge"
	"github.com/buidangphuc/team-gateway/internal/upstream"
)

type mdOrder struct {
	orderv1.OrderServiceClient
	calls int
	md    metadata.MD
}

func (m *mdOrder) CreateOrder(ctx context.Context, _ *orderv1.CreateOrderRequest, _ ...grpc.CallOption) (*orderv1.CreateOrderResponse, error) {
	m.calls++
	m.md, _ = metadata.FromOutgoingContext(ctx)
	return &orderv1.CreateOrderResponse{}, nil
}

func newHeaderFixture(t *testing.T) (orderv1connect.OrderServiceClient, *mdOrder) {
	t.Helper()
	up := &mdOrder{}
	e := edge.NewEdge(nil, []string{"listing.read"}, time.Second, 0, 1000, 1000)
	srv := httptest.NewServer(edge.NewMux(&upstream.Clients{Order: up}, e, nil, edge.CockpitConfig{}, slog.New(slog.NewTextHandler(io.Discard, nil))))
	t.Cleanup(srv.Close)
	return orderv1connect.NewOrderServiceClient(srv.Client(), srv.URL), up
}

func TestIdempotencyKeyForwardedAndValidated(t *testing.T) {
	client, up := newHeaderFixture(t)
	call := func(key string) error {
		req := connect.NewRequest(&orderv1.CreateOrderRequest{})
		if key != "" {
			req.Header().Set("Idempotency-Key", key)
		}
		_, err := client.CreateOrder(context.Background(), req)
		return err
	}

	if err := call("order-abc_123"); err != nil {
		t.Fatalf("valid key rejected: %v", err)
	}
	if got := up.md.Get("idempotency-key"); len(got) != 1 || got[0] != "order-abc_123" {
		t.Fatalf("idempotency-key metadata = %v, want [order-abc_123]", got)
	}

	up.calls, up.md = 0, nil
	if err := call(""); err != nil {
		t.Fatalf("absent key rejected: %v", err)
	}
	if got := up.md.Get("idempotency-key"); len(got) != 0 {
		t.Fatalf("absent header must add no metadata, got %v", got)
	}

	for name, key := range map[string]string{
		"too long":    strings.Repeat("k", 256),
		"non-ascii":   "khóa",
		"blank space": "   ",
	} {
		up.calls = 0
		err := call(key)
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("%s: code = %v, want invalid_argument (%v)", name, connect.CodeOf(err), err)
		}
		if up.calls != 0 {
			t.Errorf("%s: invalid key reached upstream", name)
		}
	}
}

func TestRequestIDIsValidatedBeforeForwarding(t *testing.T) {
	client, up := newHeaderFixture(t)
	call := func(rid string) {
		req := connect.NewRequest(&orderv1.CreateOrderRequest{})
		req.Header().Set("X-Request-Id", rid)
		if _, err := client.CreateOrder(context.Background(), req); err != nil {
			t.Fatal(err)
		}
	}

	call("req-123.abc_Z")
	if got := up.md.Get("x-request-id"); len(got) != 1 || got[0] != "req-123.abc_Z" {
		t.Fatalf("safe id must pass through, got %v", got)
	}
	for _, bad := range []string{"has space", "a\"b", strings.Repeat("x", 65), "khóa", "<script>"} {
		call(bad)
		got := up.md.Get("x-request-id")
		if len(got) != 1 || got[0] == bad || len(got[0]) != 32 {
			t.Errorf("unsafe id %q forwarded as %v, want a fresh generated id", bad, got)
		}
	}
}
