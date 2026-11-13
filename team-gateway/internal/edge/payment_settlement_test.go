package edge_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	paymentv1 "github.com/buidangphuc/team-gateway/generated/platform/payment/v1"
	"github.com/buidangphuc/team-gateway/generated/platform/payment/v1/paymentv1connect"
	"github.com/buidangphuc/team-gateway/internal/edge"
	"github.com/buidangphuc/team-gateway/internal/token"
	"github.com/buidangphuc/team-gateway/internal/upstream"
)

type settlementPayment struct {
	paymentv1.PaymentServiceClient
	spy      *upstreamSpy
	got      *paymentv1.GetOrderSettlementRequest
	resp     *paymentv1.GetOrderSettlementResponse
	err      error
	feeCalls atomic.Int64
}

func (s *settlementPayment) GetOrderSettlement(ctx context.Context, in *paymentv1.GetOrderSettlementRequest, _ ...grpc.CallOption) (*paymentv1.GetOrderSettlementResponse, error) {
	s.spy.record(ctx)
	s.got = in
	return s.resp, s.err
}

func (s *settlementPayment) ChargeSellerFee(context.Context, *paymentv1.ChargeSellerFeeRequest, ...grpc.CallOption) (*paymentv1.ChargeSellerFeeResponse, error) {
	s.feeCalls.Add(1)
	return &paymentv1.ChargeSellerFeeResponse{}, nil
}

func settlementHarness(t *testing.T) (*settlementPayment, *edge.Edge, string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	jwks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kty": "RSA", "use": "sig", "alg": "RS256", "kid": "kid-1",
			"n": b64(key.N.Bytes()),
			"e": b64(big.NewInt(int64(key.PublicKey.E)).Bytes()),
		}}})
	}))
	t.Cleanup(jwks.Close)
	tok := mint(t, key, "kid-1", time.Now().Add(time.Hour))
	up := &settlementPayment{spy: &upstreamSpy{}}
	e := edge.NewEdge(token.NewVerifier(jwks.URL, time.Minute), []string{"payment.read"}, time.Second, 0, 1000, 1000)
	return up, e, tok
}

func settlementClient(t *testing.T, up *settlementPayment, e *edge.Edge) paymentv1connect.PaymentServiceClient {
	t.Helper()
	opts := connect.WithInterceptors(e.Interceptors(slog.New(slog.DiscardHandler))...)
	mux := http.NewServeMux()
	p, h := paymentv1connect.NewPaymentServiceHandler(edge.NewPaymentForwarder(up, e), opts)
	mux.Handle(p, h)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return paymentv1connect.NewPaymentServiceClient(srv.Client(), srv.URL)
}

// Scenario: The seller reads the breakdown of a settled order
func TestGetOrderSettlementForwardsSellerReadAndBreakdown(t *testing.T) {
	up, e, tok := settlementHarness(t)
	c := settlementClient(t, up, e)
	up.resp = &paymentv1.GetOrderSettlementResponse{
		OrderId: "o-1", Credited: 310000, Commission: 14000, Net: 296000, CommissionRateBps: 500,
	}

	req := connect.NewRequest(&paymentv1.GetOrderSettlementRequest{OrderId: "o-1"})
	req.Header().Set("Authorization", "Bearer "+tok)
	res, err := c.GetOrderSettlement(context.Background(), req)
	if err != nil {
		t.Fatalf("GetOrderSettlement: want forwarded, got %v (code %v)", err, connect.CodeOf(err))
	}
	if up.spy.calls != 1 || up.got.GetOrderId() != "o-1" {
		t.Fatalf("not forwarded: calls=%d req=%v", up.spy.calls, up.got)
	}
	m := res.Msg
	if m.GetCredited() != 310000 || m.GetCommission() != 14000 || m.GetCommissionReversed() != 0 ||
		m.GetRefundDeducted() != 0 || m.GetNet() != 296000 || m.GetCommissionRateBps() != 500 {
		t.Fatalf("breakdown not passed through: %v", m)
	}
	if len(up.spy.md) == 0 {
		t.Fatalf("caller principal/token not forwarded: %v", up.spy.md)
	}

	for _, tc := range []struct {
		grpc    codes.Code
		connect connect.Code
	}{
		{codes.NotFound, connect.CodeNotFound},
		{codes.InvalidArgument, connect.CodeInvalidArgument},
	} {
		up.resp, up.err = nil, status.Error(tc.grpc, "x")
		req := connect.NewRequest(&paymentv1.GetOrderSettlementRequest{OrderId: "o-1"})
		req.Header().Set("Authorization", "Bearer "+tok)
		if _, err := c.GetOrderSettlement(context.Background(), req); connect.CodeOf(err) != tc.connect {
			t.Errorf("upstream %v -> %v, want %v", tc.grpc, connect.CodeOf(err), tc.connect)
		}
	}
}

// Scenario: An unauthenticated call is refused
func TestGetOrderSettlementUnauthenticatedIsRefused(t *testing.T) {
	up, e, _ := settlementHarness(t)
	c := settlementClient(t, up, e)
	up.resp = &paymentv1.GetOrderSettlementResponse{OrderId: "o-1"}

	_, err := c.GetOrderSettlement(context.Background(), connect.NewRequest(&paymentv1.GetOrderSettlementRequest{OrderId: "o-1"}))
	if connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("anonymous: got %v, want unauthenticated", err)
	}
	if up.spy.calls != 0 {
		t.Fatalf("team-payment reached by anonymous caller: %d calls", up.spy.calls)
	}
}

// Scenario: A client cannot set or forge a commission
func TestChargeSellerFeeIsNotRoutedAtTheGateway(t *testing.T) {
	f := newCockpitFixture(t)
	pay := &settlementPayment{spy: &upstreamSpy{}}
	srv := httptest.NewServer(edge.NewMux(&upstream.Clients{Payment: pay}, f.edge, nil, edge.CockpitConfig{}, slog.New(slog.DiscardHandler)))
	t.Cleanup(srv.Close)
	for _, auth := range []string{"", "Bearer " + f.token(t, "seller"), "Bearer " + f.token(t, "admin")} {
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/platform.payment.v1.PaymentService/ChargeSellerFee", strings.NewReader(`{}`))
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
			t.Fatalf("ChargeSellerFee (authenticated=%v) = %d %s, want 501 unimplemented", auth != "", res.StatusCode, b)
		}
	}
	if n := pay.feeCalls.Load(); n != 0 {
		t.Fatalf("payment upstream received %d ChargeSellerFee calls, want 0", n)
	}
}
