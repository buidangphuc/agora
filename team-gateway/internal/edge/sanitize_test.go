package edge_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	orderv1 "github.com/buidangphuc/team-gateway/generated/platform/order/v1"
	"github.com/buidangphuc/team-gateway/generated/platform/order/v1/orderv1connect"
	"github.com/buidangphuc/team-gateway/internal/edge"
	"github.com/buidangphuc/team-gateway/internal/upstream"
)

const rawSQL = `pq: duplicate key value violates unique constraint "orders_pkey" (host db-internal-7:5432)`

type failingOrder struct {
	orderv1.OrderServiceClient
	err error
}

func (f *failingOrder) GetOrder(context.Context, *orderv1.GetOrderRequest, ...grpc.CallOption) (*orderv1.GetOrderResponse, error) {
	return nil, f.err
}

func TestUpstreamErrorMessagesAreSanitisedAtTheEdge(t *testing.T) {
	cases := []struct {
		name     string
		up       error
		wantCode connect.Code
		wantMsg  string
	}{
		{"internal", status.Error(codes.Internal, rawSQL), connect.CodeInternal, "internal error"},
		{"unknown", status.Error(codes.Unknown, rawSQL), connect.CodeInternal, "internal error"},
		{"data_loss", status.Error(codes.DataLoss, rawSQL), connect.CodeDataLoss, "internal error"},
		{"unavailable", status.Error(codes.Unavailable, "dial tcp db-internal-7:5432: refused"), connect.CodeUnavailable, "service unavailable"},
		{"deadline_exceeded", status.Error(codes.DeadlineExceeded, "received context error while waiting for new LB policy update: context deadline exceeded"), connect.CodeDeadlineExceeded, "upstream timed out"},
		{"non_status", errors.New(rawSQL), connect.CodeInternal, "internal error"},
		// client-meaningful codes keep the upstream message verbatim
		{"invalid_argument", status.Error(codes.InvalidArgument, "amount must be positive"), connect.CodeInvalidArgument, "amount must be positive"},
		{"not_found", status.Error(codes.NotFound, "order not found"), connect.CodeNotFound, "order not found"},
		{"permission_denied", status.Error(codes.PermissionDenied, "not your order"), connect.CodePermissionDenied, "not your order"},
		{"failed_precondition", status.Error(codes.FailedPrecondition, "insufficient wallet balance"), connect.CodeFailedPrecondition, "insufficient wallet balance"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var logs bytes.Buffer
			logger := slog.New(slog.NewTextHandler(&logs, nil))
			e := edge.NewEdge(nil, []string{"listing.read"}, time.Second, 0, 1000, 1000)
			srv := httptest.NewServer(edge.NewMux(&upstream.Clients{Order: &failingOrder{err: c.up}}, e, nil, edge.CockpitConfig{}, logger))
			t.Cleanup(srv.Close)
			client := orderv1connect.NewOrderServiceClient(srv.Client(), srv.URL)
			req := connect.NewRequest(&orderv1.GetOrderRequest{})
			req.Header().Set("X-Request-Id", "rid-"+c.name)

			_, err := client.GetOrder(context.Background(), req)
			var ce *connect.Error
			if !errors.As(err, &ce) {
				t.Fatalf("want connect error, got %v", err)
			}
			if ce.Code() != c.wantCode || ce.Message() != c.wantMsg {
				t.Fatalf("got %v %q, want %v %q", ce.Code(), ce.Message(), c.wantCode, c.wantMsg)
			}
			if strings.Contains(ce.Message(), "db-internal") || strings.Contains(ce.Message(), "pq:") || strings.Contains(ce.Message(), "LB policy") {
				t.Fatalf("raw upstream text leaked: %q", ce.Message())
			}
			logged := strings.Contains(logs.String(), "edge.upstream_error")
			sanitised := c.wantMsg == "internal error" || c.wantMsg == "service unavailable" || c.wantMsg == "upstream timed out"
			if sanitised != logged {
				t.Fatalf("edge.upstream_error logged=%v, want %v: %s", logged, sanitised, logs.String())
			}
			if sanitised && !strings.Contains(logs.String(), "rid-"+c.name) {
				t.Fatalf("log line missing request id: %s", logs.String())
			}
			if sanitised && c.name != "unavailable" && c.name != "deadline_exceeded" && !strings.Contains(logs.String(), "db-internal") {
				t.Fatalf("log line must carry the original error: %s", logs.String())
			}
		})
	}
}
