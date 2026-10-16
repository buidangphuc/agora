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
)

type confirmOrder struct {
	orderv1.OrderServiceClient
	spy  *upstreamSpy
	got  *orderv1.ConfirmReceiptRequest
	resp *orderv1.ConfirmReceiptResponse
	err  error
}

func (s *confirmOrder) ConfirmReceipt(ctx context.Context, in *orderv1.ConfirmReceiptRequest, _ ...grpc.CallOption) (*orderv1.ConfirmReceiptResponse, error) {
	s.spy.record(ctx)
	s.got = in
	return s.resp, s.err
}

func confirmHarness(t *testing.T) (*confirmOrder, orderv1connect.OrderServiceClient, string) {
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

	up := &confirmOrder{spy: &upstreamSpy{}}
	e := edge.NewEdge(token.NewVerifier(jwks.URL, time.Minute), []string{"order.read"}, time.Second, 0, 1000, 1000)
	opts := connect.WithInterceptors(e.Interceptors(slog.New(slog.NewTextHandler(io.Discard, nil)))...)
	mux := http.NewServeMux()
	p, h := orderv1connect.NewOrderServiceHandler(edge.NewOrderForwarder(up, e), opts)
	mux.Handle(p, h)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return up, orderv1connect.NewOrderServiceClient(srv.Client(), srv.URL), tok
}

// Scenario "Order receipt: Confirm is forwarded by the gateway".
func TestConfirmReceiptForwarded(t *testing.T) {
	up, c, tok := confirmHarness(t)
	up.resp = &orderv1.ConfirmReceiptResponse{Order: &orderv1.Order{Id: "o-1", Status: orderv1.OrderStatus_ORDER_STATUS_COMPLETED}}

	req := connect.NewRequest(&orderv1.ConfirmReceiptRequest{Id: "o-1"})
	req.Header().Set("Authorization", "Bearer "+tok)
	res, err := c.ConfirmReceipt(context.Background(), req)
	if err != nil {
		t.Fatalf("ConfirmReceipt: want forwarded, got %v (code %v)", err, connect.CodeOf(err))
	}
	if up.spy.calls != 1 || up.got.GetId() != "o-1" {
		t.Fatalf("not forwarded: calls=%d req=%v", up.spy.calls, up.got)
	}
	if res.Msg.GetOrder().GetId() != "o-1" || res.Msg.GetOrder().GetStatus() != orderv1.OrderStatus_ORDER_STATUS_COMPLETED {
		t.Fatalf("response not passed through: %v", res.Msg)
	}
	if len(up.spy.md) == 0 {
		t.Fatalf("caller principal/token not forwarded: %v", up.spy.md)
	}

	for _, tc := range []struct {
		grpc    codes.Code
		connect connect.Code
	}{
		{codes.PermissionDenied, connect.CodePermissionDenied},
		{codes.NotFound, connect.CodeNotFound},
		{codes.FailedPrecondition, connect.CodeFailedPrecondition},
	} {
		up.resp, up.err = nil, status.Error(tc.grpc, "x")
		req := connect.NewRequest(&orderv1.ConfirmReceiptRequest{Id: "o-1"})
		req.Header().Set("Authorization", "Bearer "+tok)
		if _, err := c.ConfirmReceipt(context.Background(), req); connect.CodeOf(err) != tc.connect {
			t.Errorf("upstream %v -> %v, want %v", tc.grpc, connect.CodeOf(err), tc.connect)
		}
	}
}

// Scenario "Order receipt: Anonymous caller cannot confirm receipt".
func TestConfirmReceiptRequiresToken(t *testing.T) {
	up, c, _ := confirmHarness(t)
	up.resp = &orderv1.ConfirmReceiptResponse{Order: &orderv1.Order{Id: "o-1"}}

	_, err := c.ConfirmReceipt(context.Background(), connect.NewRequest(&orderv1.ConfirmReceiptRequest{Id: "o-1"}))
	if connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("anonymous: got %v, want unauthenticated", err)
	}
	if up.spy.calls != 0 {
		t.Fatalf("upstream reached by anonymous caller: %d calls", up.spy.calls)
	}
}
