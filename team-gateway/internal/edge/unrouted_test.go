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
			"n": b64(key.PublicKey.N.Bytes()),
			"e": b64(big.NewInt(int64(key.PublicKey.E)).Bytes()),
		}}})
	}))
	t.Cleanup(jwks.Close)
	userToken := mint(t, key, "kid-1", time.Now().Add(time.Hour))

	e := edge.NewEdge(token.NewVerifier(jwks.URL, time.Minute), []string{"listing.read"}, time.Second, 0, 1000, 1000)
	clients := &upstream.Clients{Listing: hitListing{}, Voucher: hitVoucher{}, Audit: hitAudit{}, AI: hitAI{}}
	srv := httptest.NewServer(edge.NewMux(clients, e, nil, edge.CockpitConfig{}, slog.New(slog.NewTextHandler(io.Discard, nil))))
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
