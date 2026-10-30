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

	"google.golang.org/grpc"

	aiv1 "github.com/buidangphuc/team-gateway/generated/platform/ai/v1"
	auditv1 "github.com/buidangphuc/team-gateway/generated/platform/audit/v1"
	identityv1 "github.com/buidangphuc/team-gateway/generated/platform/identity/v1"
	listingv1 "github.com/buidangphuc/team-gateway/generated/platform/listing/v1"
	promotionv1 "github.com/buidangphuc/team-gateway/generated/platform/promotion/v1"
	"github.com/buidangphuc/team-gateway/internal/edge"
	"github.com/buidangphuc/team-gateway/internal/token"
	"github.com/buidangphuc/team-gateway/internal/upstream"
)

var upstreamHits atomic.Int64

type hitListing struct{ listingv1.ListingServiceClient }

func (hitListing) ReserveStock(context.Context, *listingv1.ReserveStockRequest, ...grpc.CallOption) (*listingv1.ReserveStockResponse, error) {
	upstreamHits.Add(1)
	return &listingv1.ReserveStockResponse{}, nil
}

func (hitListing) ReleaseStock(context.Context, *listingv1.ReleaseStockRequest, ...grpc.CallOption) (*listingv1.ReleaseStockResponse, error) {
	upstreamHits.Add(1)
	return &listingv1.ReleaseStockResponse{}, nil
}

func (hitListing) CommitReservation(context.Context, *listingv1.CommitReservationRequest, ...grpc.CallOption) (*listingv1.CommitReservationResponse, error) {
	upstreamHits.Add(1)
	return &listingv1.CommitReservationResponse{}, nil
}

type hitVoucher struct {
	promotionv1.VoucherServiceClient
}

func (hitVoucher) CommitReservation(context.Context, *promotionv1.CommitReservationRequest, ...grpc.CallOption) (*promotionv1.CommitReservationResponse, error) {
	upstreamHits.Add(1)
	return &promotionv1.CommitReservationResponse{}, nil
}

func (hitVoucher) ReleaseReservation(context.Context, *promotionv1.ReleaseReservationRequest, ...grpc.CallOption) (*promotionv1.ReleaseReservationResponse, error) {
	upstreamHits.Add(1)
	return &promotionv1.ReleaseReservationResponse{}, nil
}

func (hitVoucher) ValidateAndReserve(context.Context, *promotionv1.ValidateAndReserveRequest, ...grpc.CallOption) (*promotionv1.ValidateAndReserveResponse, error) {
	upstreamHits.Add(1)
	return &promotionv1.ValidateAndReserveResponse{}, nil
}

type hitAI struct{ aiv1.AIServiceClient }

func (hitAI) ClassifyTags(context.Context, *aiv1.ClassifyTagsRequest, ...grpc.CallOption) (*aiv1.ClassifyTagsResponse, error) {
	upstreamHits.Add(1)
	return &aiv1.ClassifyTagsResponse{}, nil
}

type hitAudit struct{ auditv1.AuditServiceClient }

func (hitAudit) WriteAuditEvent(context.Context, *auditv1.WriteAuditEventRequest, ...grpc.CallOption) (*auditv1.WriteAuditEventResponse, error) {
	upstreamHits.Add(1)
	return &auditv1.WriteAuditEventResponse{}, nil
}

// Internal saga / audit RPCs are service-to-service only: for every caller the
// edge answers `unimplemented` (HTTP 501) and the upstream is never contacted.
// ValidateAndReserve stays routed (checkout preview).
func TestInternalRPCsAreNotRoutedAtTheEdge(t *testing.T) {
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
	userToken := mint(t, key, "kid-1", time.Now().Add(time.Hour))

	e := edge.NewEdge(token.NewVerifier(jwks.URL, time.Minute), []string{"listing.read"}, time.Second, 0, 1000, 1000)
	clients := &upstream.Clients{Listing: hitListing{}, Voucher: hitVoucher{}, Audit: hitAudit{}, AI: hitAI{}}
	srv := httptest.NewServer(edge.NewMux(clients, e, nil, edge.CockpitConfig{}, slog.New(slog.DiscardHandler)))
	t.Cleanup(srv.Close)

	authHeader := ""
	post := func(path string) (int, string) {
		req, err := http.NewRequest(http.MethodPost, srv.URL+path, strings.NewReader(`{}`))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		if authHeader != "" {
			req.Header.Set("Authorization", authHeader)
		}
		res, err := srv.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		return res.StatusCode, string(b)
	}

	upstreamHits.Store(0)
	for _, caller := range []struct{ name, auth string }{
		{"anonymous", ""},
		{"authenticated", "Bearer " + userToken},
	} {
		authHeader = caller.auth
		for _, path := range []string{
			"/platform.listing.v1.ListingService/ReserveStock",
			"/platform.listing.v1.ListingService/ReleaseStock",
			"/platform.listing.v1.ListingService/CommitReservation",
			"/platform.promotion.v1.VoucherService/CommitReservation",
			"/platform.promotion.v1.VoucherService/ReleaseReservation",
			"/platform.audit.v1.AuditService/WriteAuditEvent",
			"/platform.ai.v1.AIService/ClassifyTags",
		} {
			if code, body := post(path); code != http.StatusNotImplemented || !strings.Contains(body, "unimplemented") {
				t.Errorf("%s %s = %d %s, want 501 unimplemented", caller.name, path, code, body)
			}
		}
	}
	authHeader = ""
	if n := upstreamHits.Load(); n != 0 {
		t.Fatalf("upstream received %d calls through unrouted RPCs, want 0", n)
	}

	if code, body := post("/platform.promotion.v1.VoucherService/ValidateAndReserve"); code != http.StatusOK {
		t.Fatalf("ValidateAndReserve = %d %s, want forwarded 200", code, body)
	}
	if n := upstreamHits.Load(); n != 1 {
		t.Fatalf("ValidateAndReserve upstream calls = %d, want 1", n)
	}
}

// Scenario: Procedure is covered by the unrouted guard.
func TestUnroutedGuardIncludesRefreshSession(t *testing.T) {
	id := &fakeRefreshIdentity{}
	e := edge.NewEdge(token.NewVerifier("http://127.0.0.1:1/jwks", time.Minute), []string{"listing.read"}, time.Second, 0, 1000, 1000)
	srv := httptest.NewServer(edge.NewMux(&upstream.Clients{Identity: id}, e, nil, edge.CockpitConfig{}, slog.New(slog.DiscardHandler)))
	t.Cleanup(srv.Close)
	res, err := srv.Client().Post(srv.URL+"/platform.identity.v1.AuthService/RefreshSession", "application/json", strings.NewReader(`{"refreshToken":"rt-1"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	if res.StatusCode == http.StatusNotImplemented || strings.Contains(string(b), "unimplemented") {
		t.Fatalf("RefreshSession = %d %s, want routed (not unimplemented)", res.StatusCode, b)
	}
	if id.calls.Load() != 1 {
		t.Fatalf("identity calls = %d, want 1", id.calls.Load())
	}
}

type screenAI struct {
	aiv1.AIServiceClient
	calls atomic.Int64
}

func (s *screenAI) ScreenListing(context.Context, *aiv1.ScreenListingRequest, ...grpc.CallOption) (*aiv1.ScreenListingResponse, error) {
	s.calls.Add(1)
	return &aiv1.ScreenListingResponse{}, nil
}

// Scenario: Edge: ScreenListing is not routed. Every caller, admin included,
// gets 501 unimplemented and the upstream is never contacted.
func TestUnroutedScreenListing(t *testing.T) {
	f := newCockpitFixture(t)
	ai := &screenAI{}
	srv := httptest.NewServer(edge.NewMux(&upstream.Clients{AI: ai}, f.edge, nil, edge.CockpitConfig{}, slog.New(slog.DiscardHandler)))
	t.Cleanup(srv.Close)
	for _, auth := range []string{"", "Bearer " + f.token(t, "admin")} {
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/platform.ai.v1.AIService/ScreenListing", strings.NewReader(`{}`))
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
			t.Fatalf("ScreenListing (authenticated=%v) = %d %s, want 501 unimplemented", auth != "", res.StatusCode, b)
		}
	}
	if n := ai.calls.Load(); n != 0 {
		t.Fatalf("upstream received %d ScreenListing calls, want 0", n)
	}
}

// Scenario: User enforcement edge: Procedures are covered by the unrouted guard.
// Every AccountAdminService method in the generated descriptor is routed.
func TestUnroutedGuardAccountAdmin(t *testing.T) {
	svc := identityv1.File_platform_identity_v1_identity_proto.Services().ByName("AccountAdminService")
	if svc == nil || svc.Methods().Len() < 2 {
		t.Fatalf("AccountAdminService descriptor missing or incomplete")
	}
	for i := 0; i < svc.Methods().Len(); i++ {
		path := "/platform.identity.v1.AccountAdminService/" + string(svc.Methods().Get(i).Name())
		up := &accountAdminUpstream{}
		code, body := callAccountAdmin(t, up, path, `{"user_id":"u-9","status":"USER_STATUS_BANNED","reason_code":"fraud"}`)
		if code == http.StatusNotImplemented || strings.Contains(body, "unimplemented") {
			t.Errorf("%s = %d %s, want routed", path, code, body)
		}
		if up.setCalls.Load()+up.listCalls.Load() != 1 {
			t.Errorf("%s: upstream calls = %d, want 1", path, up.setCalls.Load()+up.listCalls.Load())
		}
	}
}

// Scenario: Dispute edge: The new RPCs are not left unrouted.
func TestUnroutedGuardCoversDisputeLists(t *testing.T) {
	admin := func(f *cockpitFixture) string { return "Bearer " + f.token(t, "admin") }
	for _, c := range []struct {
		name, path, body string
		calls            func(*disputeUpstream) int64
	}{
		{"ListOrderDisputes", listOrderDisputesPath, `{"order_id":"o-7"}`, func(u *disputeUpstream) int64 { return u.listOrderCalls.Load() }},
		{"ListDisputes", listDisputesPath, `{}`, func(u *disputeUpstream) int64 { return u.listCalls.Load() }},
	} {
		up := &disputeUpstream{}
		code, body := callDisputeRPC(t, up, c.path, c.body, admin)
		if code != http.StatusOK {
			t.Fatalf("admin %s = %d %s, want forwarded 200 (not 501)", c.name, code, body)
		}
		if n := c.calls(up); n != 1 {
			t.Fatalf("%s upstream calls = %d, want 1", c.name, n)
		}
	}
}
