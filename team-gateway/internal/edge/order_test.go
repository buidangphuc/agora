package edge_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http/httptest"
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

type feeOrder struct {
	orderv1.OrderServiceClient
	calls int
	got   *orderv1.CalculateShippingFeeRequest
	resp  *orderv1.CalculateShippingFeeResponse
	err   error
}

func (f *feeOrder) CalculateShippingFee(_ context.Context, in *orderv1.CalculateShippingFeeRequest, _ ...grpc.CallOption) (*orderv1.CalculateShippingFeeResponse, error) {
	f.calls++
	f.got = in
	return f.resp, f.err
}

func feeClient(t *testing.T, up *feeOrder) orderv1connect.OrderServiceClient {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	e := edge.NewEdge(nil, []string{"order.read"}, time.Second, 0, 1000, 1000)
	srv := httptest.NewServer(edge.NewMux(&upstream.Clients{Order: up}, e, nil, edge.CockpitConfig{}, logger))
	t.Cleanup(srv.Close)
	return orderv1connect.NewOrderServiceClient(srv.Client(), srv.URL)
}

// Scenario: Advertised price: the gateway forwards the shipping fee quote
func TestOrderForwarderCalculateShippingFee(t *testing.T) {
	up := &feeOrder{resp: &orderv1.CalculateShippingFeeResponse{ShippingFee: 20000, IsFreeShipping: false}}
	c := feeClient(t, up)

	res, err := c.CalculateShippingFee(context.Background(), connect.NewRequest(&orderv1.CalculateShippingFeeRequest{City: "Hà Nội", ItemsSubtotal: 350000}))
	if err != nil {
		t.Fatalf("CalculateShippingFee: %v (code %v)", err, connect.CodeOf(err))
	}
	if res.Msg.GetShippingFee() != 20000 || res.Msg.GetIsFreeShipping() {
		t.Fatalf("response not passed through: %v", res.Msg)
	}
	if up.calls != 1 || up.got.GetCity() != "Hà Nội" || up.got.GetItemsSubtotal() != 350000 {
		t.Fatalf("order client got calls=%d req=%v", up.calls, up.got)
	}
}

// Scenario: Advertised price: an order-service error on the fee quote is passed through
func TestOrderForwarderCalculateShippingFeeError(t *testing.T) {
	up := &feeOrder{err: status.Error(codes.Unavailable, "dial tcp db-internal-7:5432: refused")}
	c := feeClient(t, up)

	res, err := c.CalculateShippingFee(context.Background(), connect.NewRequest(&orderv1.CalculateShippingFeeRequest{City: "Hà Nội", ItemsSubtotal: 350000}))
	var ce *connect.Error
	if !errors.As(err, &ce) || ce.Code() != connect.CodeUnavailable {
		t.Fatalf("want connect unavailable, got %v", err)
	}
	if res != nil {
		t.Fatalf("no fee expected, got %v", res.Msg)
	}
	if up.calls != 1 {
		t.Fatalf("order client calls=%d, want 1", up.calls)
	}
}

// Scenario: Advertised price: the fee quote needs no buyer scope
func TestOrderForwarderCalculateShippingFeeAnonymous(t *testing.T) {
	up := &feeOrder{resp: &orderv1.CalculateShippingFeeResponse{ShippingFee: 35000}}
	c := feeClient(t, up)

	res, err := c.CalculateShippingFee(context.Background(), connect.NewRequest(&orderv1.CalculateShippingFeeRequest{City: "Cần Thơ", ItemsSubtotal: 100000}))
	if err != nil {
		code := connect.CodeOf(err)
		if code == connect.CodeUnauthenticated || code == connect.CodePermissionDenied {
			t.Fatalf("anonymous quote must not need a buyer scope: %v", err)
		}
		t.Fatalf("CalculateShippingFee: %v (code %v)", err, code)
	}
	if res.Msg.GetShippingFee() != 35000 {
		t.Fatalf("quote not returned: %v", res.Msg)
	}
	if up.calls != 1 || up.got.GetCity() != "Cần Thơ" || up.got.GetItemsSubtotal() != 100000 {
		t.Fatalf("order client got calls=%d req=%v", up.calls, up.got)
	}
}
