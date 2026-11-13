package edge_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"log/slog"
	"math/big"
	"net/http"
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
	"github.com/buidangphuc/team-gateway/internal/token"
)

type bundleCart struct {
	orderv1.CartServiceClient
	spy  *upstreamSpy
	got  *orderv1.AddBundleToCartRequest
	resp *orderv1.AddBundleToCartResponse
	err  error
}

func (s *bundleCart) AddBundleToCart(ctx context.Context, in *orderv1.AddBundleToCartRequest, _ ...grpc.CallOption) (*orderv1.AddBundleToCartResponse, error) {
	s.spy.record(ctx)
	s.got = in
	return s.resp, s.err
}

func bundleHarness(t *testing.T) (*bundleCart, orderv1connect.CartServiceClient, string, *syncBuf) {
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

	logs := &syncBuf{}
	up := &bundleCart{spy: &upstreamSpy{}}
	e := edge.NewEdge(token.NewVerifier(jwks.URL, time.Minute), []string{"order.read"}, time.Second, 0, 1000, 1000)
	opts := connect.WithInterceptors(e.Interceptors(slog.New(slog.NewJSONHandler(logs, nil)))...)
	mux := http.NewServeMux()
	p, h := orderv1connect.NewCartServiceHandler(edge.NewCartForwarder(up, e), opts)
	mux.Handle(p, h)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return up, orderv1connect.NewCartServiceClient(srv.Client(), srv.URL), tok, logs
}

// Scenario "An unauthenticated caller is rejected" (forwarding of the authenticated principal).
func TestBundleAddToCartForwardsPrincipal(t *testing.T) {
	up, c, tok, _ := bundleHarness(t)
	up.resp = &orderv1.AddBundleToCartResponse{Cart: &orderv1.Cart{}}

	req := connect.NewRequest(&orderv1.AddBundleToCartRequest{BundleId: "bnd-1", Quantity: 2})
	req.Header().Set("Authorization", "Bearer "+tok)
	res, err := c.AddBundleToCart(context.Background(), req)
	if err != nil {
		t.Fatalf("AddBundleToCart: want forwarded, got %v (code %v)", err, connect.CodeOf(err))
	}
	if up.spy.calls != 1 || up.got.GetBundleId() != "bnd-1" || up.got.GetQuantity() != 2 {
		t.Fatalf("not forwarded: calls=%d req=%v", up.spy.calls, up.got)
	}
	if res.Msg.GetCart() == nil {
		t.Fatalf("response not passed through: %v", res.Msg)
	}
	if len(up.spy.md) == 0 {
		t.Fatalf("caller principal/token not forwarded: %v", up.spy.md)
	}
}

// Scenario "An unauthenticated caller is rejected".
func TestBundleAddToCartRejectsUnauthenticated(t *testing.T) {
	up, c, _, _ := bundleHarness(t)
	up.resp = &orderv1.AddBundleToCartResponse{Cart: &orderv1.Cart{}}

	_, err := c.AddBundleToCart(context.Background(), connect.NewRequest(&orderv1.AddBundleToCartRequest{BundleId: "bnd-1", Quantity: 1}))
	if connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("anonymous: got %v, want unauthenticated", err)
	}
	if up.spy.calls != 0 {
		t.Fatalf("upstream reached by anonymous caller: %d calls", up.spy.calls)
	}
}

// Scenario "An unauthenticated caller is rejected" (upstream errors keep their code).
func TestBundleAddToCartMapsUpstreamErrors(t *testing.T) {
	up, c, tok, _ := bundleHarness(t)
	for _, tc := range []struct {
		grpc    codes.Code
		connect connect.Code
	}{
		{codes.NotFound, connect.CodeNotFound},
		{codes.FailedPrecondition, connect.CodeFailedPrecondition},
		{codes.InvalidArgument, connect.CodeInvalidArgument},
		{codes.Unauthenticated, connect.CodeUnauthenticated},
	} {
		up.resp, up.err = nil, status.Error(tc.grpc, "x")
		req := connect.NewRequest(&orderv1.AddBundleToCartRequest{BundleId: "bnd-1", Quantity: 1})
		req.Header().Set("Authorization", "Bearer "+tok)
		if _, err := c.AddBundleToCart(context.Background(), req); connect.CodeOf(err) != tc.connect {
			t.Errorf("upstream %v -> %v, want %v", tc.grpc, connect.CodeOf(err), tc.connect)
		}
	}
}

// Scenario "An unauthenticated caller is rejected" (no raw ids in logs).
func TestBundleAddToCartLogsNoRawIDs(t *testing.T) {
	up, c, tok, logs := bundleHarness(t)
	up.resp = &orderv1.AddBundleToCartResponse{Cart: &orderv1.Cart{}}

	req := connect.NewRequest(&orderv1.AddBundleToCartRequest{BundleId: "bnd-secret-77", Quantity: 1})
	req.Header().Set("Authorization", "Bearer "+tok)
	if _, err := c.AddBundleToCart(context.Background(), req); err != nil {
		t.Fatalf("AddBundleToCart: %v", err)
	}
	out := logs.String()
	if strings.TrimSpace(out) == "" {
		t.Fatalf("no edge log line written for the call")
	}
	for _, raw := range []string{"bnd-secret-77", "user-1"} {
		if strings.Contains(out, raw) {
			t.Errorf("raw id %q leaked into logs: %s", raw, out)
		}
	}
}
