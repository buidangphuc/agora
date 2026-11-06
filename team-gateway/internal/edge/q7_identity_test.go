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
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/timestamppb"

	identityv1 "github.com/buidangphuc/team-gateway/generated/platform/identity/v1"
	"github.com/buidangphuc/team-gateway/generated/platform/identity/v1/identityv1connect"
	"github.com/buidangphuc/team-gateway/internal/edge"
	"github.com/buidangphuc/team-gateway/internal/token"
)

type q7IdentitySpy struct {
	identityv1.AuthServiceClient
	registerCalls atomic.Int32
	accountCalls  atomic.Int32
	mu            sync.Mutex
	registerReq   *identityv1.RegisterRequest
	accountMD     metadata.MD
}

func (s *q7IdentitySpy) Register(_ context.Context, in *identityv1.RegisterRequest, _ ...grpc.CallOption) (*identityv1.RegisterResponse, error) {
	s.registerCalls.Add(1)
	s.mu.Lock()
	s.registerReq = in
	s.mu.Unlock()
	return &identityv1.RegisterResponse{Result: &identityv1.AuthResult{}}, nil
}

func (s *q7IdentitySpy) GetMyAccount(ctx context.Context, _ *identityv1.GetMyAccountRequest, _ ...grpc.CallOption) (*identityv1.GetMyAccountResponse, error) {
	s.accountCalls.Add(1)
	md, _ := metadata.FromOutgoingContext(ctx)
	s.mu.Lock()
	s.accountMD = md
	s.mu.Unlock()
	id := ""
	if v := md.Get("x-principal-id"); len(v) == 1 {
		id = v[0]
	}
	return &identityv1.GetMyAccountResponse{UserId: id, CreatedAt: timestamppb.New(time.Unix(1700000000, 0))}, nil
}

type q7IdentityFixture struct {
	client identityv1connect.AuthServiceClient
	spy    *q7IdentitySpy
	key    *rsa.PrivateKey
}

func newQ7IdentityFixture(t *testing.T) *q7IdentityFixture {
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

	spy := &q7IdentitySpy{}
	e := edge.NewEdge(token.NewVerifier(jwks.URL, time.Minute), []string{"listing.read"}, time.Second, 3, 1000, 1000)
	opts := connect.WithInterceptors(e.Interceptors(slog.New(slog.DiscardHandler))...)
	mux := http.NewServeMux()
	p, h := identityv1connect.NewAuthServiceHandler(edge.NewAuthForwarder(spy, e), opts)
	mux.Handle(p, h)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return &q7IdentityFixture{client: identityv1connect.NewAuthServiceClient(srv.Client(), srv.URL), spy: spy, key: key}
}

func (f *q7IdentityFixture) bearer(t *testing.T, sub string, scopes []string) string {
	t.Helper()
	claims := &token.Claims{
		Type:      "user",
		Scopes:    scopes,
		SessionID: "sid-1",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   sub,
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

func (f *q7IdentityFixture) getMyAccount(auth string, headers map[string]string) (*identityv1.GetMyAccountResponse, error) {
	req := connect.NewRequest(&identityv1.GetMyAccountRequest{})
	if auth != "" {
		req.Header().Set("Authorization", auth)
	}
	for k, v := range headers {
		req.Header().Set(k, v)
	}
	res, err := f.client.GetMyAccount(context.Background(), req)
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

// Scenario: Gateway forwards the field
func TestQ7RegisterForwardsReferralCodeUnchanged(t *testing.T) {
	for _, code := range []string{"AB12cd", " AB12 ", "bad code;DROP TABLE", "ABé12", ""} {
		f := newQ7IdentityFixture(t)
		_, err := f.client.Register(context.Background(), connect.NewRequest(&identityv1.RegisterRequest{
			Username: "alice", Password: "password123", Role: "buyer", ReferralCode: code,
		}))
		if err != nil {
			t.Fatalf("Register(%q): %v", code, err)
		}
		if n := f.spy.registerCalls.Load(); n != 1 {
			t.Fatalf("identity Register calls = %d, want 1", n)
		}
		if got := f.spy.registerReq.GetReferralCode(); got != code {
			t.Fatalf("referral_code forwarded = %q, want %q", got, code)
		}
		if f.spy.registerReq.GetUsername() != "alice" || f.spy.registerReq.GetRole() != "buyer" {
			t.Fatalf("other Register fields altered: %+v", f.spy.registerReq)
		}
	}
}

// Scenario: Register stays public at the edge
func TestQ7RegisterStaysPublicWithReferralCode(t *testing.T) {
	f := newQ7IdentityFixture(t)
	_, err := f.client.Register(context.Background(), connect.NewRequest(&identityv1.RegisterRequest{
		Username: "bob", Password: "password123", ReferralCode: "AB12cd",
	}))
	if code := connect.CodeOf(err); err != nil && (code == connect.CodeUnauthenticated || code == connect.CodePermissionDenied) {
		t.Fatalf("anonymous Register = %v, must not be refused", err)
	}
	if err != nil {
		t.Fatalf("anonymous Register: %v", err)
	}
}

// Scenario: A seller and a buyer are both allowed
func TestQ7GetMyAccountAllowsBuyerAndSeller(t *testing.T) {
	cases := []struct {
		name   string
		id     string
		scopes []string
	}{
		{"buyer", "buyer-1", nil},
		{"buyer-with-scope", "buyer-2", []string{"order.read"}},
		{"seller", "seller-1", []string{"seller"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newQ7IdentityFixture(t)
			res, err := f.getMyAccount(f.bearer(t, tc.id, tc.scopes), nil)
			if err != nil {
				t.Fatalf("GetMyAccount: %v", err)
			}
			if res.GetUserId() != tc.id {
				t.Fatalf("user_id = %q, want %q", res.GetUserId(), tc.id)
			}
			if n := f.spy.accountCalls.Load(); n != 1 {
				t.Fatalf("identity calls = %d, want 1", n)
			}
		})
	}
}

// Scenario: An anonymous call is refused
func TestQ7GetMyAccountAnonymousIs401(t *testing.T) {
	cases := map[string]map[string]string{
		"no-token":       nil,
		"spoofed-header": {"X-Principal-Id": "user-1", "X-Principal-Type": "user"},
	}
	for name, hdr := range cases {
		t.Run(name, func(t *testing.T) {
			f := newQ7IdentityFixture(t)
			_, err := f.getMyAccount("", hdr)
			if connect.CodeOf(err) != connect.CodeUnauthenticated {
				t.Fatalf("anonymous GetMyAccount = %v, want unauthenticated", err)
			}
			if n := f.spy.accountCalls.Load(); n != 0 {
				t.Fatalf("identity calls = %d, want 0", n)
			}
		})
	}
}

// Scenario: An anonymous call is refused (invalid token)
func TestQ7GetMyAccountInvalidTokenIs401(t *testing.T) {
	f := newQ7IdentityFixture(t)
	_, err := f.getMyAccount("Bearer not-a-jwt", nil)
	if connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("invalid-token GetMyAccount = %v, want unauthenticated", err)
	}
	if n := f.spy.accountCalls.Load(); n != 0 {
		t.Fatalf("identity calls = %d, want 0", n)
	}
}

// Scenario: A caller can never read another user's account (principal forwarded unchanged, spoofed id overridden)
func TestQ7GetMyAccountForwardsVerifiedPrincipalOnly(t *testing.T) {
	f := newQ7IdentityFixture(t)
	res, err := f.getMyAccount(f.bearer(t, "buyer-B", []string{"order.read"}), map[string]string{
		"X-Principal-Id": "buyer-A",
		"X-User-Id":      "buyer-A",
	})
	if err != nil {
		t.Fatalf("GetMyAccount: %v", err)
	}
	if res.GetUserId() != "buyer-B" {
		t.Fatalf("user_id = %q, want buyer-B", res.GetUserId())
	}
	md := f.spy.accountMD
	if got := md.Get("x-principal-id"); len(got) != 1 || got[0] != "buyer-B" {
		t.Fatalf("x-principal-id = %v, want [buyer-B]", got)
	}
	if got := md.Get("x-principal-type"); len(got) != 1 || got[0] != "user" {
		t.Fatalf("x-principal-type = %v, want [user]", got)
	}
}
