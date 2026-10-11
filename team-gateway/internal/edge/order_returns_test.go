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

type returnsOrder struct {
	orderv1.OrderServiceClient
	spy  *upstreamSpy
	got  *orderv1.ListOrderReturnsRequest
	resp *orderv1.ListOrderReturnsResponse
	err  error
}

func (s *returnsOrder) ListOrderReturns(ctx context.Context, in *orderv1.ListOrderReturnsRequest, _ ...grpc.CallOption) (*orderv1.ListOrderReturnsResponse, error) {
	s.spy.record(ctx)
	s.got = in
	return s.resp, s.err
}

// ListOrderReturns is a pure forwarder: routed (not 501), the request passes
// through unchanged with the caller's principal, and upstream codes come back
// unchanged.
func TestListOrderReturnsIsRoutedAndForwardedUnchanged(t *testing.T) {
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

	up := &returnsOrder{spy: &upstreamSpy{}}
	e := edge.NewEdge(token.NewVerifier(jwks.URL, time.Minute), []string{"order.read"}, time.Second, 0, 1000, 1000)
	opts := connect.WithInterceptors(e.Interceptors(slog.New(slog.NewTextHandler(io.Discard, nil)))...)
	mux := http.NewServeMux()
	p, h := orderv1connect.NewOrderServiceHandler(edge.NewOrderForwarder(up, e), opts)
	mux.Handle(p, h)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := orderv1connect.NewOrderServiceClient(srv.Client(), srv.URL)

	call := func() (*connect.Response[orderv1.ListOrderReturnsResponse], error) {
		req := connect.NewRequest(&orderv1.ListOrderReturnsRequest{OrderId: "o-1"})
		req.Header().Set("Authorization", "Bearer "+tok)
		return c.ListOrderReturns(context.Background(), req)
	}

	up.resp = &orderv1.ListOrderReturnsResponse{Returns: []*orderv1.OrderReturn{{Id: "r-1"}, {Id: "r-2"}}}
	res, err := call()
	if err != nil {
		t.Fatalf("ListOrderReturns: %v", err)
	}
	if up.got.GetOrderId() != "o-1" || len(res.Msg.GetReturns()) != 2 || res.Msg.Returns[0].Id != "r-1" {
		t.Fatalf("not passed through: req=%v resp=%v", up.got, res.Msg)
	}
	if up.spy.md.Get("x-principal-id") == nil && len(up.spy.md) == 0 {
		t.Fatalf("caller principal not forwarded: %v", up.spy.md)
	}

	for _, tc := range []struct {
		grpc    codes.Code
		connect connect.Code
	}{
		{codes.PermissionDenied, connect.CodePermissionDenied},
		{codes.NotFound, connect.CodeNotFound},
	} {
		up.resp, up.err = nil, status.Error(tc.grpc, "x")
		if _, err := call(); connect.CodeOf(err) != tc.connect {
			t.Errorf("upstream %v -> %v, want %v", tc.grpc, connect.CodeOf(err), tc.connect)
		}
	}
}
