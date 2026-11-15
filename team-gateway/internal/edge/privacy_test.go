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

	identityv1 "github.com/buidangphuc/team-gateway/generated/platform/identity/v1"
	"github.com/buidangphuc/team-gateway/generated/platform/identity/v1/identityv1connect"
	"github.com/buidangphuc/team-gateway/internal/edge"
	"github.com/buidangphuc/team-gateway/internal/token"
)

type privacySpy struct {
	identityv1.PrivacyServiceClient
	recordCalls atomic.Int32
	listCalls   atomic.Int32
	otherCalls  atomic.Int32
	mu          sync.Mutex
	md          metadata.MD
	recordReq   *identityv1.RecordConsentRequest
}

func (s *privacySpy) RecordConsent(ctx context.Context, in *identityv1.RecordConsentRequest, _ ...grpc.CallOption) (*identityv1.RecordConsentResponse, error) {
	s.recordCalls.Add(1)
	s.mu.Lock()
	s.md, _ = metadata.FromOutgoingContext(ctx)
	s.recordReq = in
	s.mu.Unlock()
	return &identityv1.RecordConsentResponse{}, nil
}

func (s *privacySpy) ListMyConsents(_ context.Context, _ *identityv1.ListMyConsentsRequest, _ ...grpc.CallOption) (*identityv1.ListMyConsentsResponse, error) {
	s.listCalls.Add(1)
	return &identityv1.ListMyConsentsResponse{}, nil
}

func (s *privacySpy) RequestAccountDeletion(context.Context, *identityv1.RequestAccountDeletionRequest, ...grpc.CallOption) (*identityv1.RequestAccountDeletionResponse, error) {
	s.otherCalls.Add(1)
	return &identityv1.RequestAccountDeletionResponse{}, nil
}

func (s *privacySpy) CancelAccountDeletion(context.Context, *identityv1.CancelAccountDeletionRequest, ...grpc.CallOption) (*identityv1.CancelAccountDeletionResponse, error) {
	s.otherCalls.Add(1)
	return &identityv1.CancelAccountDeletionResponse{}, nil
}

func (s *privacySpy) GetAccountDeletionStatus(context.Context, *identityv1.GetAccountDeletionStatusRequest, ...grpc.CallOption) (*identityv1.GetAccountDeletionStatusResponse, error) {
	s.otherCalls.Add(1)
	return &identityv1.GetAccountDeletionStatusResponse{}, nil
}

func (s *privacySpy) ExportMyData(context.Context, *identityv1.ExportMyDataRequest, ...grpc.CallOption) (*identityv1.ExportMyDataResponse, error) {
	s.otherCalls.Add(1)
	return &identityv1.ExportMyDataResponse{}, nil
}

type privacyFixture struct {
	client identityv1connect.PrivacyServiceClient
	spy    *privacySpy
	key    *rsa.PrivateKey
}

func newPrivacyFixture(t *testing.T) *privacyFixture {
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

	spy := &privacySpy{}
	e := edge.NewEdge(token.NewVerifier(jwks.URL, time.Minute), []string{"listing.read"}, time.Second, 3, 1000, 1000)
	opts := connect.WithInterceptors(e.Interceptors(slog.New(slog.DiscardHandler))...)
	mux := http.NewServeMux()
	p, h := identityv1connect.NewPrivacyServiceHandler(edge.NewPrivacyForwarder(spy, e), opts)
	mux.Handle(p, h)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return &privacyFixture{client: identityv1connect.NewPrivacyServiceClient(srv.Client(), srv.URL), spy: spy, key: key}
}

func (f *privacyFixture) userBearer(t *testing.T) string {
	t.Helper()
	claims := &token.Claims{
		Type:   "user",
		Scopes: []string{"order.read"},
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

func privacyReq[T any](msg *T, auth string) *connect.Request[T] {
	r := connect.NewRequest(msg)
	if auth != "" {
		r.Header().Set("Authorization", auth)
	}
	return r
}

// Scenario: Consent is reachable through the gateway (RecordConsent forwarded with principal metadata).
func TestPrivacyForwarderRecordConsentForwarded(t *testing.T) {
	f := newPrivacyFixture(t)
	in := &identityv1.RecordConsentRequest{Records: []*identityv1.ConsentRecord{{
		Purpose: identityv1.ConsentPurpose_CONSENT_PURPOSE_ANALYTICS, Granted: true,
	}}}
	if _, err := f.client.RecordConsent(context.Background(), privacyReq(in, f.userBearer(t))); err != nil {
		t.Fatalf("RecordConsent: %v", err)
	}
	if n := f.spy.recordCalls.Load(); n != 1 {
		t.Fatalf("identity RecordConsent calls = %d, want 1", n)
	}
	f.spy.mu.Lock()
	defer f.spy.mu.Unlock()
	if len(f.spy.recordReq.GetRecords()) != 1 {
		t.Fatalf("forwarded records = %d, want 1", len(f.spy.recordReq.GetRecords()))
	}
	if got := f.spy.md.Get("x-principal-id"); len(got) != 1 || got[0] != "user-1" {
		t.Fatalf("x-principal-id forwarded = %v, want [user-1]", got)
	}
}

// Scenario: Consent is reachable through the gateway (ListMyConsents forwarded).
func TestPrivacyForwarderListMyConsentsForwarded(t *testing.T) {
	f := newPrivacyFixture(t)
	if _, err := f.client.ListMyConsents(context.Background(), privacyReq(&identityv1.ListMyConsentsRequest{}, f.userBearer(t))); err != nil {
		t.Fatalf("ListMyConsents: %v", err)
	}
	if n := f.spy.listCalls.Load(); n != 1 {
		t.Fatalf("identity ListMyConsents calls = %d, want 1", n)
	}
}

// Scenario: Anonymous edge call is refused (RecordConsent without a bearer token).
func TestPrivacyForwarderAnonymousRecordConsentRefused(t *testing.T) {
	f := newPrivacyFixture(t)
	_, err := f.client.RecordConsent(context.Background(), privacyReq(&identityv1.RecordConsentRequest{}, ""))
	if connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("anonymous RecordConsent = %v, want unauthenticated", err)
	}
	if n := f.spy.recordCalls.Load(); n != 0 {
		t.Fatalf("identity calls = %d, want 0", n)
	}
}

// Scenario: Anonymous edge call is refused (ListMyConsents without a bearer token).
func TestPrivacyForwarderAnonymousListRefused(t *testing.T) {
	f := newPrivacyFixture(t)
	_, err := f.client.ListMyConsents(context.Background(), privacyReq(&identityv1.ListMyConsentsRequest{}, ""))
	if connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("anonymous ListMyConsents = %v, want unauthenticated", err)
	}
	if n := f.spy.listCalls.Load(); n != 0 {
		t.Fatalf("identity calls = %d, want 0", n)
	}
}

// Scenario: Anonymous edge call is refused (spoofed principal headers stay anonymous).
func TestPrivacyForwarderSpoofedHeadersRefused(t *testing.T) {
	f := newPrivacyFixture(t)
	r := privacyReq(&identityv1.RecordConsentRequest{}, "")
	r.Header().Set("X-Principal-Id", "user-1")
	r.Header().Set("X-Principal-Type", "user")
	_, err := f.client.RecordConsent(context.Background(), r)
	if connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("spoofed RecordConsent = %v, want unauthenticated", err)
	}
	if n := f.spy.recordCalls.Load(); n != 0 {
		t.Fatalf("identity calls = %d, want 0", n)
	}
}
