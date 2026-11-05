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
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	identityv1 "github.com/buidangphuc/team-gateway/generated/platform/identity/v1"
	"github.com/buidangphuc/team-gateway/generated/platform/identity/v1/identityv1connect"
	orderv1 "github.com/buidangphuc/team-gateway/generated/platform/order/v1"
	"github.com/buidangphuc/team-gateway/generated/platform/order/v1/orderv1connect"
	searchv1 "github.com/buidangphuc/team-gateway/generated/platform/search/v1"
	"github.com/buidangphuc/team-gateway/generated/platform/search/v1/searchv1connect"
	"github.com/buidangphuc/team-gateway/internal/edge"
	"github.com/buidangphuc/team-gateway/internal/token"
)

type mdRecorder struct {
	mu sync.Mutex
	md map[string]metadata.MD
}

func (r *mdRecorder) put(ctx context.Context, name string) {
	md, _ := metadata.FromOutgoingContext(ctx)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.md[name] = md
}

func (r *mdRecorder) get(t *testing.T, name string) metadata.MD {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	md, ok := r.md[name]
	if !ok {
		t.Fatalf("%s did not reach the upstream", name)
	}
	return md
}

type sidIdentity struct {
	identityv1.AuthServiceClient
	rec *mdRecorder
}

func (s sidIdentity) Login(ctx context.Context, _ *identityv1.LoginRequest, _ ...grpc.CallOption) (*identityv1.LoginResponse, error) {
	s.rec.put(ctx, "Login")
	return &identityv1.LoginResponse{}, nil
}

func (s sidIdentity) Register(ctx context.Context, _ *identityv1.RegisterRequest, _ ...grpc.CallOption) (*identityv1.RegisterResponse, error) {
	s.rec.put(ctx, "Register")
	return &identityv1.RegisterResponse{}, nil
}

func (s sidIdentity) RefreshSession(ctx context.Context, _ *identityv1.RefreshSessionRequest, _ ...grpc.CallOption) (*identityv1.RefreshSessionResponse, error) {
	s.rec.put(ctx, "RefreshSession")
	return &identityv1.RefreshSessionResponse{}, nil
}

func (s sidIdentity) BecomeSeller(ctx context.Context, _ *identityv1.BecomeSellerRequest, _ ...grpc.CallOption) (*identityv1.BecomeSellerResponse, error) {
	s.rec.put(ctx, "BecomeSeller")
	return &identityv1.BecomeSellerResponse{Result: &identityv1.AuthResult{}}, nil
}

func (s sidIdentity) CreateHandoffCode(ctx context.Context, _ *identityv1.CreateHandoffCodeRequest, _ ...grpc.CallOption) (*identityv1.CreateHandoffCodeResponse, error) {
	s.rec.put(ctx, "CreateHandoffCode")
	return &identityv1.CreateHandoffCodeResponse{}, nil
}

func (s sidIdentity) PeekHandoffCode(ctx context.Context, _ *identityv1.PeekHandoffCodeRequest, _ ...grpc.CallOption) (*identityv1.PeekHandoffCodeResponse, error) {
	s.rec.put(ctx, "PeekHandoffCode")
	return &identityv1.PeekHandoffCodeResponse{}, nil
}

func (s sidIdentity) RedeemHandoffCode(ctx context.Context, _ *identityv1.RedeemHandoffCodeRequest, _ ...grpc.CallOption) (*identityv1.RedeemHandoffCodeResponse, error) {
	s.rec.put(ctx, "RedeemHandoffCode")
	return &identityv1.RedeemHandoffCodeResponse{}, nil
}

type sidSearch struct {
	searchv1.SearchServiceClient
	rec *mdRecorder
}

func (s sidSearch) SearchListings(ctx context.Context, _ *searchv1.SearchListingsRequest, _ ...grpc.CallOption) (*searchv1.SearchListingsResponse, error) {
	s.rec.put(ctx, "SearchListings")
	return &searchv1.SearchListingsResponse{}, nil
}

type sidOrder struct {
	orderv1.OrderServiceClient
	rec *mdRecorder
}

func (s sidOrder) GetOrder(ctx context.Context, _ *orderv1.GetOrderRequest, _ ...grpc.CallOption) (*orderv1.GetOrderResponse, error) {
	s.rec.put(ctx, "GetOrder")
	return &orderv1.GetOrderResponse{}, nil
}

type sidFixture struct {
	rec    *mdRecorder
	key    *rsa.PrivateKey
	auth   identityv1connect.AuthServiceClient
	search searchv1connect.SearchServiceClient
	order  orderv1connect.OrderServiceClient
}

func newSidFixture(t *testing.T) *sidFixture {
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

	rec := &mdRecorder{md: map[string]metadata.MD{}}
	e := edge.NewEdge(token.NewVerifier(jwks.URL, time.Minute), []string{"listing.read", "search:read"}, time.Second, 0, 1000, 1000)
	opts := connect.WithInterceptors(e.Interceptors(slog.New(slog.DiscardHandler))...)
	mux := http.NewServeMux()
	ap, ah := identityv1connect.NewAuthServiceHandler(edge.NewAuthForwarder(sidIdentity{rec: rec}, e), opts)
	mux.Handle(ap, ah)
	sp, sh := searchv1connect.NewSearchServiceHandler(edge.NewSearchForwarder(sidSearch{rec: rec}, e), opts)
	mux.Handle(sp, sh)
	op, oh := orderv1connect.NewOrderServiceHandler(edge.NewOrderForwarder(sidOrder{rec: rec}, e), opts)
	mux.Handle(op, oh)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return &sidFixture{
		rec:    rec,
		key:    key,
		auth:   identityv1connect.NewAuthServiceClient(srv.Client(), srv.URL),
		search: searchv1connect.NewSearchServiceClient(srv.Client(), srv.URL),
		order:  orderv1connect.NewOrderServiceClient(srv.Client(), srv.URL),
	}
}

func (f *sidFixture) bearer(t *testing.T, sid string) string {
	t.Helper()
	claims := &token.Claims{
		Type:      "user",
		Scopes:    []string{"order.read"},
		SessionID: sid,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user-1",
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-time.Minute)),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = "kid-1"
	s, err := tok.SignedString(f.key)
	if err != nil {
		t.Fatal(err)
	}
	return "Bearer " + s
}

func withHeaders[T any](msg *T, hdr map[string]string) *connect.Request[T] {
	req := connect.NewRequest(msg)
	for k, v := range hdr {
		req.Header().Set(k, v)
	}
	return req
}

func (f *sidFixture) call(t *testing.T, name string, hdr map[string]string) metadata.MD {
	t.Helper()
	ctx := context.Background()
	var err error
	switch name {
	case "Login":
		_, err = f.auth.Login(ctx, withHeaders(&identityv1.LoginRequest{}, hdr))
	case "Register":
		_, err = f.auth.Register(ctx, withHeaders(&identityv1.RegisterRequest{}, hdr))
	case "RefreshSession":
		_, err = f.auth.RefreshSession(ctx, withHeaders(&identityv1.RefreshSessionRequest{}, hdr))
	case "BecomeSeller":
		_, err = f.auth.BecomeSeller(ctx, withHeaders(&identityv1.BecomeSellerRequest{TermsVersion: "v1"}, hdr))
	case "CreateHandoffCode":
		_, err = f.auth.CreateHandoffCode(ctx, withHeaders(&identityv1.CreateHandoffCodeRequest{}, hdr))
	case "PeekHandoffCode":
		_, err = f.auth.PeekHandoffCode(ctx, withHeaders(&identityv1.PeekHandoffCodeRequest{}, hdr))
	case "RedeemHandoffCode":
		_, err = f.auth.RedeemHandoffCode(ctx, withHeaders(&identityv1.RedeemHandoffCodeRequest{}, hdr))
	case "SearchListings":
		_, err = f.search.SearchListings(ctx, withHeaders(&searchv1.SearchListingsRequest{}, hdr))
	case "GetOrder":
		_, err = f.order.GetOrder(ctx, withHeaders(&orderv1.GetOrderRequest{}, hdr))
	default:
		t.Fatalf("unknown procedure %s", name)
	}
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return f.rec.get(t, name)
}

func wantPrincipal(t *testing.T, name string, md metadata.MD) {
	t.Helper()
	if got := md.Get("x-principal-id"); len(got) != 1 || got[0] != "user-1" {
		t.Fatalf("%s x-principal-id = %v, want [user-1]", name, got)
	}
	if got := md.Get("x-principal-type"); len(got) != 1 || got[0] != "user" {
		t.Fatalf("%s x-principal-type = %v, want [user]", name, got)
	}
	if got := md.Get("x-principal-scopes"); len(got) != 1 || got[0] != "order.read" {
		t.Fatalf("%s x-principal-scopes = %v, want [order.read]", name, got)
	}
}

// Scenario: Session id is forwarded to BecomeSeller and CreateHandoffCode
func TestOutgoingForwardsSidOnlyToAllowlist(t *testing.T) {
	f := newSidFixture(t)
	auth := map[string]string{"Authorization": f.bearer(t, "sid-verified"), "X-Principal-Sid": "sid-forged"}
	for _, name := range []string{"BecomeSeller", "CreateHandoffCode", "PeekHandoffCode", "RedeemHandoffCode"} {
		t.Run(name, func(t *testing.T) {
			md := f.call(t, name, auth)
			if got := md.Get("x-principal-sid"); len(got) != 1 || got[0] != "sid-verified" {
				t.Fatalf("%s x-principal-sid = %v, want [sid-verified]", name, got)
			}
			wantPrincipal(t, name, md)
		})
	}
	t.Run("token without sid", func(t *testing.T) {
		md := f.call(t, "BecomeSeller", map[string]string{"Authorization": f.bearer(t, ""), "X-Principal-Sid": "sid-forged"})
		if got := md.Get("x-principal-sid"); len(got) != 0 {
			t.Fatalf("x-principal-sid = %v, want none", got)
		}
	})
}

// Scenario: Session id is never forwarded elsewhere
// Scenario: A client-supplied sid header is dropped
func TestOutgoingOmitsSidOutsideAllowlist(t *testing.T) {
	f := newSidFixture(t)
	auth := map[string]string{"Authorization": f.bearer(t, "sid-verified"), "X-Principal-Sid": "sid-forged"}
	for _, name := range []string{"Login", "Register", "RefreshSession", "SearchListings", "GetOrder"} {
		t.Run(name, func(t *testing.T) {
			md := f.call(t, name, auth)
			if got := md.Get("x-principal-sid"); len(got) != 0 {
				t.Fatalf("%s x-principal-sid = %v, want none", name, got)
			}
			wantPrincipal(t, name, md)
		})
	}
	anon := map[string]string{"X-Principal-Sid": "sid-forged"}
	for _, name := range []string{"PeekHandoffCode", "RedeemHandoffCode", "Login", "SearchListings"} {
		t.Run("anonymous "+name, func(t *testing.T) {
			md := f.call(t, name, anon)
			if got := md.Get("x-principal-sid"); len(got) != 0 {
				t.Fatalf("anonymous %s x-principal-sid = %v, want none", name, got)
			}
		})
	}
}
