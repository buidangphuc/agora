package edge_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"google.golang.org/grpc"

	orderv1 "github.com/buidangphuc/team-gateway/generated/platform/order/v1"
	"github.com/buidangphuc/team-gateway/internal/edge"
	"github.com/buidangphuc/team-gateway/internal/upstream"
)

type q7HasCompletedOrder struct {
	orderv1.OrderServiceClient
	calls atomic.Int64
}

func (s *q7HasCompletedOrder) HasCompletedOrder(context.Context, *orderv1.HasCompletedOrderRequest, ...grpc.CallOption) (*orderv1.HasCompletedOrderResponse, error) {
	s.calls.Add(1)
	return &orderv1.HasCompletedOrderResponse{HasCompleted: true}, nil
}

// Scenario: Not routed at the gateway
func TestQ7HasCompletedOrderIsNotRoutedAtTheGateway(t *testing.T) {
	f := newCockpitFixture(t)
	ord := &q7HasCompletedOrder{}
	srv := httptest.NewServer(edge.NewMux(&upstream.Clients{Order: ord}, f.edge, nil, edge.CockpitConfig{}, slog.New(slog.DiscardHandler)))
	t.Cleanup(srv.Close)
	for _, auth := range []string{"", "Bearer " + f.token(t, "buyer"), "Bearer " + f.token(t, "admin")} {
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/platform.order.v1.OrderService/HasCompletedOrder", strings.NewReader(`{"buyer_id":"u-1"}`))
		req.Header.Set("Content-Type", "application/json")
		if auth != "" {
			req.Header.Set("Authorization", auth)
		}
		res, err := srv.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode != http.StatusNotImplemented || !strings.Contains(string(b), "unimplemented") {
			t.Fatalf("HasCompletedOrder (authenticated=%v) = %d %s, want 501 unimplemented", auth != "", res.StatusCode, b)
		}
	}
	if n := ord.calls.Load(); n != 0 {
		t.Fatalf("order upstream received %d HasCompletedOrder calls, want 0", n)
	}
}
