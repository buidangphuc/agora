package listing_test

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	promotionv1 "github.com/buidangphuc/team-domain/generated/platform/promotion/v1"
	"github.com/buidangphuc/team-domain/internal/listing"
)

type fakeSubscriptions struct {
	promotionv1.UnimplementedSubscriptionServiceServer
	mu     sync.Mutex
	resp   *promotionv1.GetEntitlementsResponse
	err    error
	block  bool
	sellID []string
}

func (f *fakeSubscriptions) GetEntitlements(ctx context.Context, req *promotionv1.GetEntitlementsRequest) (*promotionv1.GetEntitlementsResponse, error) {
	f.mu.Lock()
	f.sellID = append(f.sellID, req.GetSellerId())
	f.mu.Unlock()
	if f.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if f.err != nil {
		return nil, f.err
	}
	return f.resp, nil
}

func startSubscriptions(t *testing.T, f *fakeSubscriptions) listing.EntitlementsPort {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := grpc.NewServer()
	promotionv1.RegisterSubscriptionServiceServer(srv, f)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return listing.NewGRPCEntitlements(promotionv1.NewSubscriptionServiceClient(conn))
}

func limits(kv map[string]string) *promotionv1.GetEntitlementsResponse {
	return &promotionv1.GetEntitlementsResponse{Plan: promotionv1.PlanTier_PLAN_TIER_FREE, Limits: kv}
}

func TestEntitlementsAdapterParsesLimits(t *testing.T) {
	cases := []struct {
		name         string
		limits       map[string]string
		wantMax      int64
		wantFeatured int64
	}{
		{"finite", map[string]string{"max_listings": "10", "featured_listings": "3"}, 10, 3},
		{"unlimited is -1", map[string]string{"max_listings": "-1", "featured_listings": "-1"}, -1, -1},
		{"zero cap", map[string]string{"max_listings": "0", "featured_listings": "0"}, 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeSubscriptions{resp: limits(tc.limits)}
			port := startSubscriptions(t, f)
			got, err := port.Get(context.Background(), "seller-1")
			if err != nil {
				t.Fatalf("Get: %v", err)
			}
			if got.MaxListings != tc.wantMax || got.FeaturedListings != tc.wantFeatured {
				t.Errorf("got %+v, want max=%d featured=%d", got, tc.wantMax, tc.wantFeatured)
			}
			if len(f.sellID) != 1 || f.sellID[0] != "seller-1" {
				t.Errorf("upstream saw seller ids %v, want [seller-1]", f.sellID)
			}
		})
	}
}

func TestCreateFailsClosedOnMalformedLimits(t *testing.T) {
	cases := map[string]map[string]string{
		"missing max_listings key": {"featured_listings": "3"},
		"empty limits":             {},
		"non-integer value":        {"max_listings": "abc", "featured_listings": "3"},
		"blank value":              {"max_listings": " ", "featured_listings": "3"},
		"decimal value":            {"max_listings": "1.5", "featured_listings": "3"},
	}
	for name, kv := range cases {
		t.Run(name, func(t *testing.T) {
			port := startSubscriptions(t, &fakeSubscriptions{resp: limits(kv)})
			got, err := port.Get(context.Background(), "seller-1")
			if err == nil {
				t.Fatalf("malformed limits must be an error, got %+v", got)
			}
			if !errors.Is(err, listing.ErrEntitlementsUnavailable) {
				t.Errorf("err = %v, want errors.Is ErrEntitlementsUnavailable", err)
			}
		})
	}
}

func TestCreateFailsClosedOnUpstreamTimeout(t *testing.T) {
	port := startSubscriptions(t, &fakeSubscriptions{block: true})
	start := time.Now()
	_, err := port.Get(context.Background(), "seller-1")
	if err == nil {
		t.Fatal("an unanswered upstream must be an error")
	}
	if !errors.Is(err, listing.ErrEntitlementsUnavailable) {
		t.Errorf("err = %v, want errors.Is ErrEntitlementsUnavailable", err)
	}
	if d := time.Since(start); d > 3*time.Second {
		t.Errorf("returned after %v, want within 3s", d)
	}
}
