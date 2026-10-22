package edge_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"math/big"
	"net/http"
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
	"github.com/buidangphuc/team-gateway/internal/token"
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

type previewOrder struct {
	orderv1.OrderServiceClient
	spy  *upstreamSpy
	got  *orderv1.PreviewCheckoutRequest
	resp *orderv1.PreviewCheckoutResponse
	err  error
}

func (p *previewOrder) PreviewCheckout(ctx context.Context, in *orderv1.PreviewCheckoutRequest, _ ...grpc.CallOption) (*orderv1.PreviewCheckoutResponse, error) {
	p.spy.record(ctx)
	p.got = in
	return p.resp, p.err
}

func previewClient(t *testing.T, up *previewOrder) (orderv1connect.OrderServiceClient, string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	jwks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kty": "RSA", "use": "sig", "alg": "RS256", "kid": "kid-1",
			"n": b64(key.PublicKey.N.Bytes()),
			"e": b64(big.NewInt(int64(key.PublicKey.E)).Bytes()),
		}}})
	}))
	t.Cleanup(jwks.Close)
	tok := mint(t, key, "kid-1", time.Now().Add(time.Hour))
	e := edge.NewEdge(token.NewVerifier(jwks.URL, time.Minute), []string{"order.read"}, time.Second, 0, 1000, 1000)
	opts := connect.WithInterceptors(e.Interceptors(slog.New(slog.NewTextHandler(io.Discard, nil)))...)
	mux := http.NewServeMux()
	path, h := orderv1connect.NewOrderServiceHandler(edge.NewOrderForwarder(up, e), opts)
	mux.Handle(path, h)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return orderv1connect.NewOrderServiceClient(srv.Client(), srv.URL), tok
}

// Scenario: Multishop checkout: The gateway forwards PreviewCheckout with the caller's identity
func TestOrderForwarderPreviewCheckout(t *testing.T) {
	up := &previewOrder{spy: &upstreamSpy{}, resp: &orderv1.PreviewCheckoutResponse{
		Groups:        []*orderv1.CheckoutGroupQuote{{SellerId: "s-1", TotalAmount: 120000}, {SellerId: "s-2", TotalAmount: 80000}},
		ItemsSubtotal: 190000, ShippingFee: 30000, DiscountAmount: 20000, TotalAmount: 200000, ShippingQuoted: true,
	}}
	c, tok := previewClient(t, up)

	req := connect.NewRequest(&orderv1.PreviewCheckoutRequest{
		AddressId:           "addr-1",
		ItemIds:             []string{"i-1", "i-2"},
		PlatformVoucherCode: "SAVE10",
	})
	req.Header().Set("Authorization", "Bearer "+tok)
	res, err := c.PreviewCheckout(context.Background(), req)
	if err != nil {
		t.Fatalf("PreviewCheckout: %v (code %v)", err, connect.CodeOf(err))
	}
	if up.spy.calls != 1 || up.got.GetAddressId() != "addr-1" || len(up.got.GetItemIds()) != 2 || up.got.GetPlatformVoucherCode() != "SAVE10" {
		t.Fatalf("upstream did not get the same request: calls=%d req=%v", up.spy.calls, up.got)
	}
	if len(res.Msg.GetGroups()) != 2 || res.Msg.GetGroups()[0].GetSellerId() != "s-1" || res.Msg.GetTotalAmount() != 200000 || !res.Msg.GetShippingQuoted() {
		t.Fatalf("response not returned unchanged: %v", res.Msg)
	}
	if len(up.spy.md) == 0 {
		t.Fatalf("caller principal not forwarded: %v", up.spy.md)
	}
}

// Scenario: Multishop checkout: The gateway maps an upstream failure
func TestOrderForwarderPreviewCheckoutMapsError(t *testing.T) {
	up := &previewOrder{spy: &upstreamSpy{}, err: status.Error(codes.FailedPrecondition, "cart is empty")}
	c, tok := previewClient(t, up)

	req := connect.NewRequest(&orderv1.PreviewCheckoutRequest{AddressId: "addr-1"})
	req.Header().Set("Authorization", "Bearer "+tok)
	res, err := c.PreviewCheckout(context.Background(), req)
	var ce *connect.Error
	if !errors.As(err, &ce) || ce.Code() != connect.CodeFailedPrecondition {
		t.Fatalf("want connect failed-precondition, got %v", err)
	}
	if ce.Message() != "cart is empty" {
		t.Fatalf("message = %q, want %q", ce.Message(), "cart is empty")
	}
	if res != nil {
		t.Fatalf("no response expected, got %v", res.Msg)
	}
	if up.spy.calls != 1 {
		t.Fatalf("upstream calls=%d, want 1", up.spy.calls)
	}
}
