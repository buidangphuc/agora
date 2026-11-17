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
	paymentv1 "github.com/buidangphuc/team-gateway/generated/platform/payment/v1"
	"github.com/buidangphuc/team-gateway/internal/edge"
	"github.com/buidangphuc/team-gateway/internal/upstream"
)

type obligationsOrder struct {
	orderv1.OrderServiceClient
	calls atomic.Int64
}

func (s *obligationsOrder) GetUserObligations(context.Context, *orderv1.GetUserObligationsRequest, ...grpc.CallOption) (*orderv1.GetUserObligationsResponse, error) {
	s.calls.Add(1)
	return &orderv1.GetUserObligationsResponse{OpenBuyerOrders: 1}, nil
}

type obligationsPayment struct {
	paymentv1.PaymentServiceClient
	calls atomic.Int64
}

func (s *obligationsPayment) GetPayoutObligations(context.Context, *paymentv1.GetPayoutObligationsRequest, ...grpc.CallOption) (*paymentv1.GetPayoutObligationsResponse, error) {
	s.calls.Add(1)
	return &paymentv1.GetPayoutObligationsResponse{PendingPayouts: 1}, nil
}

func TestObligationRPCsAreNotRoutedAtTheGateway(t *testing.T) {
	f := newCockpitFixture(t)
	ord := &obligationsOrder{}
	pay := &obligationsPayment{}
	srv := httptest.NewServer(edge.NewMux(&upstream.Clients{Order: ord, Payment: pay}, f.edge, nil, edge.CockpitConfig{}, slog.New(slog.DiscardHandler)))
	t.Cleanup(srv.Close)
	for _, tc := range []struct{ scenario, path string }{
		{"OBL-10", "/platform.order.v1.OrderService/GetUserObligations"},
		{"PAY-10", "/platform.payment.v1.PaymentService/GetPayoutObligations"},
	} {
		for _, auth := range []string{"", "Bearer " + f.token(t, "buyer"), "Bearer " + f.token(t, "admin")} {
			req, _ := http.NewRequest(http.MethodPost, srv.URL+tc.path, strings.NewReader(`{"user_id":"u-1"}`))
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
			if res.StatusCode != http.StatusNotImplemented {
				t.Fatalf("%s %s (authenticated=%v) = %d %s, want 501", tc.scenario, tc.path, auth != "", res.StatusCode, b)
			}
		}
	}
	if n := ord.calls.Load(); n != 0 {
		t.Fatalf("OBL-10: order upstream received %d calls, want 0", n)
	}
	if n := pay.calls.Load(); n != 0 {
		t.Fatalf("PAY-10: payment upstream received %d calls, want 0", n)
	}
}
