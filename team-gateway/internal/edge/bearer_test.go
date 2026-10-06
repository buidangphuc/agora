package edge_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	orderv1 "github.com/buidangphuc/team-gateway/generated/platform/order/v1"
	"github.com/buidangphuc/team-gateway/generated/platform/order/v1/orderv1connect"
	searchv1 "github.com/buidangphuc/team-gateway/generated/platform/search/v1"
	"github.com/buidangphuc/team-gateway/generated/platform/search/v1/searchv1connect"
	"github.com/buidangphuc/team-gateway/internal/edge"
	"github.com/buidangphuc/team-gateway/internal/revocation"
	"github.com/buidangphuc/team-gateway/internal/token"
)

// Bearer handling at the edge (RFC 6750 §3.1, ADR-0003): no token is anonymous;
// a token that is present but does not verify is Unauthenticated on EVERY route,
// public (SearchListings) and protected (GetOrder) alike. It is never silently
// downgraded to anonymous/PUBLIC_SCOPES.

type upstreamSpy struct {
	calls int
	md    metadata.MD
}

func (u *upstreamSpy) record(ctx context.Context) {
	u.calls++
	u.md, _ = metadata.FromOutgoingContext(ctx)
}

type spySearch struct {
	searchv1.SearchServiceClient
	spy *upstreamSpy
}

func (s spySearch) SearchListings(ctx context.Context, _ *searchv1.SearchListingsRequest, _ ...grpc.CallOption) (*searchv1.SearchListingsResponse, error) {
	s.spy.record(ctx)
	return &searchv1.SearchListingsResponse{}, nil
}

type spyOrder struct {
	orderv1.OrderServiceClient
	spy *upstreamSpy
}

func (s spyOrder) GetOrder(ctx context.Context, _ *orderv1.GetOrderRequest, _ ...grpc.CallOption) (*orderv1.GetOrderResponse, error) {
	s.spy.record(ctx)
	return &orderv1.GetOrderResponse{}, nil
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func mint(t *testing.T, key *rsa.PrivateKey, kid string, exp time.Time) string {
	t.Helper()
	return mintSID(t, key, kid, exp, "")
}

// mintSID is mint with a `sid` (session id) claim.
func mintSID(t *testing.T, key *rsa.PrivateKey, kid string, exp time.Time, sid string) string {
	t.Helper()
	claims := &token.Claims{
		Type:      "user",
		Scopes:    []string{"order.read", "search:read"},
		SessionID: sid,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user-1",
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
			ExpiresAt: jwt.NewNumericDate(exp),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = kid
	s, err := tok.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

type bearerFixture struct {
	srv         *httptest.Server
	search, ord *upstreamSpy
	key         *rsa.PrivateKey
	denylist    *revocation.Denylist
}

func newBearerFixture(t *testing.T) *bearerFixture {
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

	f := &bearerFixture{search: &upstreamSpy{}, ord: &upstreamSpy{}, key: key, denylist: revocation.NewDenylist()}
	e := edge.NewEdge(token.NewVerifier(jwks.URL, time.Minute), []string{"listing.read", "search:read"}, time.Second, 0, 1000, 1000).
		WithRevocations(f.denylist)
	opts := connect.WithInterceptors(e.Interceptors(slog.New(slog.NewTextHandler(io.Discard, nil)))...)
	mux := http.NewServeMux()
	sp, sh := searchv1connect.NewSearchServiceHandler(edge.NewSearchForwarder(spySearch{spy: f.search}, e), opts)
	mux.Handle(sp, sh)
	op, oh := orderv1connect.NewOrderServiceHandler(edge.NewOrderForwarder(spyOrder{spy: f.ord}, e), opts)
	mux.Handle(op, oh)
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

// call returns the Connect error code (0 = ok) for one RPC with the given
// Authorization header ("" = header absent).
func (f *bearerFixture) call(t *testing.T, route, auth string) (connect.Code, *connect.Error) {
	t.Helper()
	var err error
	switch route {
	case "public":
		c := searchv1connect.NewSearchServiceClient(f.srv.Client(), f.srv.URL)
		req := connect.NewRequest(&searchv1.SearchListingsRequest{})
		if auth != "" {
			req.Header().Set("Authorization", auth)
		}
		_, err = c.SearchListings(context.Background(), req)
	case "protected":
		c := orderv1connect.NewOrderServiceClient(f.srv.Client(), f.srv.URL)
		req := connect.NewRequest(&orderv1.GetOrderRequest{})
		if auth != "" {
			req.Header().Set("Authorization", auth)
		}
		_, err = c.GetOrder(context.Background(), req)
	}
	if err == nil {
		return 0, nil
	}
	ce := err.(*connect.Error)
	return ce.Code(), ce
}

func (f *bearerFixture) spy(route string) *upstreamSpy {
	if route == "public" {
		return f.search
	}
	return f.ord
}

func TestBearerResolution(t *testing.T) {
	f := newBearerFixture(t)
	other, _ := rsa.GenerateKey(rand.Reader, 2048)
	hour := time.Hour

	cases := []struct {
		name      string
		auth      string
		wantCode  connect.Code // 0 = forwarded
		wantID    string
		wantScope string
	}{
		{"none", "", 0, "anonymous", "listing.read,search:read"},
		{"valid", "Bearer " + mint(t, f.key, "kid-1", time.Now().Add(hour)), 0, "user-1", "order.read,search:read"},
		{"garbage", "Bearer garbage", connect.CodeUnauthenticated, "", ""},
		{"empty bearer", "Bearer ", connect.CodeUnauthenticated, "", ""},
		{"bad signature", "Bearer " + mint(t, other, "kid-1", time.Now().Add(hour)), connect.CodeUnauthenticated, "", ""},
		{"unknown kid", "Bearer " + mint(t, f.key, "kid-nope", time.Now().Add(hour)), connect.CodeUnauthenticated, "", ""},
		{"expired", "Bearer " + mint(t, f.key, "kid-1", time.Now().Add(-time.Minute)), connect.CodeUnauthenticated, "", ""},
	}
	for _, route := range []string{"public", "protected"} {
		for _, tc := range cases {
			t.Run(route+"/"+tc.name, func(t *testing.T) {
				spy := f.spy(route)
				spy.calls, spy.md = 0, nil
				code, ce := f.call(t, route, tc.auth)
				if code != tc.wantCode {
					t.Fatalf("code = %v, want %v (err=%v)", code, tc.wantCode, ce)
				}
				if tc.wantCode != 0 {
					if spy.calls != 0 {
						t.Fatalf("invalid token must not reach upstream, got %d calls", spy.calls)
					}
					if got := ce.Meta().Get("WWW-Authenticate"); got == "" {
						t.Errorf("want WWW-Authenticate challenge on 401")
					}
					return
				}
				if spy.calls != 1 {
					t.Fatalf("want 1 upstream call, got %d", spy.calls)
				}
				if v := spy.md.Get("x-principal-id"); len(v) != 1 || v[0] != tc.wantID {
					t.Errorf("principal id = %v, want %s", v, tc.wantID)
				}
				if v := spy.md.Get("x-principal-scopes"); len(v) != 1 || v[0] != tc.wantScope {
					t.Errorf("scopes = %v, want %s", v, tc.wantScope)
				}
			})
		}
	}
}

// A token whose `sid` is denylisted is a 401 on every route (public and protected),
// with the same invalid_token challenge as any other bad bearer, and never reaches
// upstream. Other sessions of the same user, and sid-less tokens, keep working.
func TestRevokedSessionIsRejected(t *testing.T) {
	f := newBearerFixture(t)
	exp := time.Now().Add(time.Hour)
	liveTok := "Bearer " + mintSID(t, f.key, "kid-1", exp, "sid-live")
	noSIDTok := "Bearer " + mint(t, f.key, "kid-1", exp)

	for _, route := range []string{"public", "protected"} {
		t.Run(route, func(t *testing.T) {
			sid := "sid-revoked-" + route
			revokedTok := "Bearer " + mintSID(t, f.key, "kid-1", exp, sid)
			// Before the revoke event arrives the token still works (fail open).
			if code, ce := f.call(t, route, revokedTok); code != 0 {
				t.Fatalf("before revoke: code = %v (%v), want ok", code, ce)
			}
			f.denylist.Add(sid, exp)

			spy := f.spy(route)
			spy.calls, spy.md = 0, nil
			code, ce := f.call(t, route, revokedTok)
			if code != connect.CodeUnauthenticated {
				t.Fatalf("revoked sid: code = %v, want Unauthenticated", code)
			}
			if got := ce.Meta().Get("WWW-Authenticate"); got != `Bearer error="invalid_token"` {
				t.Errorf("WWW-Authenticate = %q, want Bearer error=\"invalid_token\"", got)
			}
			if spy.calls != 0 {
				t.Errorf("a revoked token must not reach upstream, got %d calls", spy.calls)
			}

			for name, tok := range map[string]string{"other session": liveTok, "no sid": noSIDTok} {
				if code, ce := f.call(t, route, tok); code != 0 {
					t.Errorf("%s: code = %v (%v), want ok", name, code, ce)
				}
			}
		})
	}
}
