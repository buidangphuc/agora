package names_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"

	identityv1 "github.com/buidangphuc/team-notification/generated/platform/identity/v1"
	listingv1 "github.com/buidangphuc/team-notification/generated/platform/listing/v1"
	"github.com/buidangphuc/team-notification/internal/names"
)

type listingFake struct {
	listingv1.UnimplementedListingServiceServer
	mu    sync.Mutex
	shops []*listingv1.ShopSummary
	err   error
	block time.Duration
	calls int
	md    metadata.MD
	req   *listingv1.BatchGetStorefrontsRequest
}

func (f *listingFake) BatchGetStorefronts(ctx context.Context, in *listingv1.BatchGetStorefrontsRequest) (*listingv1.BatchGetStorefrontsResponse, error) {
	f.mu.Lock()
	f.calls++
	f.req = in
	f.md, _ = metadata.FromIncomingContext(ctx)
	block, err, shops := f.block, f.err, f.shops
	f.mu.Unlock()
	if block > 0 {
		select {
		case <-time.After(block):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if err != nil {
		return nil, err
	}
	return &listingv1.BatchGetStorefrontsResponse{Shops: shops}, nil
}

func (f *listingFake) snapshot() (int, metadata.MD, *listingv1.BatchGetStorefrontsRequest) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls, f.md, f.req
}

type identityFake struct {
	identityv1.UnimplementedPublicProfileServiceServer
	mu       sync.Mutex
	profiles []*identityv1.PublicProfile
	err      error
	calls    int
	md       metadata.MD
}

func (f *identityFake) GetPublicProfiles(ctx context.Context, in *identityv1.GetPublicProfilesRequest) (*identityv1.GetPublicProfilesResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.md, _ = metadata.FromIncomingContext(ctx)
	if f.err != nil {
		return nil, f.err
	}
	return &identityv1.GetPublicProfilesResponse{Profiles: f.profiles}, nil
}

func (f *identityFake) snapshot() (int, metadata.MD) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls, f.md
}

func serve(t *testing.T, register func(*grpc.Server)) string {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := grpc.NewServer()
	register(srv)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	return lis.Addr().String()
}

func newResolver(t *testing.T, lf *listingFake, pf *identityFake) *names.Resolver {
	t.Helper()
	domain := serve(t, func(s *grpc.Server) { listingv1.RegisterListingServiceServer(s, lf) })
	identity := serve(t, func(s *grpc.Server) { identityv1.RegisterPublicProfileServiceServer(s, pf) })
	r, err := names.New(domain, identity, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("names.New: %v", err)
	}
	t.Cleanup(r.Close)
	return r
}

func TestResolve_SellerGetsShopName(t *testing.T) {
	lf := &listingFake{shops: []*listingv1.ShopSummary{{SellerId: "seller-1", DisplayName: "  Nhà Sách An Nhiên  "}}}
	pf := &identityFake{profiles: []*identityv1.PublicProfile{{UserId: "seller-1", DisplayName: "seller.one"}}}
	r := newResolver(t, lf, pf)

	if got := r.ResolveSenderName(context.Background(), "seller-1", "seller-1"); got != "Nhà Sách An Nhiên" {
		t.Fatalf("name = %q", got)
	}
	if n, _ := pf.snapshot(); n != 0 {
		t.Fatal("identity must not be called when the shop name resolves")
	}
	_, _, req := lf.snapshot()
	if len(req.GetSellerIds()) != 1 || req.GetSellerIds()[0] != "seller-1" {
		t.Fatalf("storefront request = %v", req)
	}
}

func TestResolve_BuyerGetsUserDisplayName(t *testing.T) {
	lf := &listingFake{shops: []*listingv1.ShopSummary{{SellerId: "buyer-1", DisplayName: "A shop the buyer happens to own"}}}
	pf := &identityFake{profiles: []*identityv1.PublicProfile{{UserId: "buyer-1", DisplayName: "an.nguyen"}}}
	r := newResolver(t, lf, pf)

	if got := r.ResolveSenderName(context.Background(), "buyer-1", "seller-1"); got != "an.nguyen" {
		t.Fatalf("name = %q", got)
	}
	if n, _, _ := lf.snapshot(); n != 0 {
		t.Fatal("a non-seller sender must not hit team-domain")
	}
}

func TestResolve_SellerWithoutShopFallsBackToProfile(t *testing.T) {
	lf := &listingFake{}
	pf := &identityFake{profiles: []*identityv1.PublicProfile{{UserId: "seller-1", DisplayName: "seller.one"}}}
	r := newResolver(t, lf, pf)

	if got := r.ResolveSenderName(context.Background(), "seller-1", "seller-1"); got != "seller.one" {
		t.Fatalf("name = %q", got)
	}
}

func TestResolve_LookupFailureYieldsEmpty(t *testing.T) {
	boom := errors.New("unavailable")
	r := newResolver(t, &listingFake{err: boom}, &identityFake{err: boom})

	if got := r.ResolveSenderName(context.Background(), "seller-1", "seller-1"); got != "" {
		t.Fatalf("both lookups failing must yield empty, got %q", got)
	}
	if got := r.ResolveSenderName(context.Background(), "buyer-1", "seller-1"); got != "" {
		t.Fatalf("profile failure must yield empty, got %q", got)
	}
	if got := r.ResolveSenderName(context.Background(), "", "seller-1"); got != "" {
		t.Fatalf("empty sender must yield empty, got %q", got)
	}
}

func TestResolve_UnreachableUpstreamYieldsEmpty(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := lis.Addr().String()
	_ = lis.Close()
	r, err := names.New(addr, addr, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("dialing is lazy, names.New must not fail: %v", err)
	}
	t.Cleanup(r.Close)
	if got := r.ResolveSenderName(context.Background(), "seller-1", "seller-1"); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestResolverSendsReadOnlyServicePrincipal(t *testing.T) {
	lf := &listingFake{}
	pf := &identityFake{profiles: []*identityv1.PublicProfile{{UserId: "seller-1", DisplayName: "seller.one"}}}
	r := newResolver(t, lf, pf)

	ctx := auth.WithPrincipal(context.Background(), auth.Principal{ID: "user-77", Type: "user", Scopes: []string{"order.write", "admin"}})
	ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs("x-principal-id", "user-77", "x-principal-type", "user", "x-principal-scopes", "admin"))
	if got := r.ResolveSenderName(ctx, "seller-1", "seller-1"); got != "seller.one" {
		t.Fatalf("name = %q", got)
	}

	_, lmd, _ := lf.snapshot()
	_, pmd := pf.snapshot()
	for label, md := range map[string]metadata.MD{"team-domain": lmd, "team-identity": pmd} {
		for key, want := range map[string]string{
			"x-principal-id":     "service-team-notification",
			"x-principal-type":   "service",
			"x-principal-scopes": "listing.read,identity.read",
		} {
			if got := md.Get(key); len(got) != 1 || got[0] != want {
				t.Errorf("%s %s = %v, want [%s]", label, key, got, want)
			}
		}
	}
}

func TestResolverBoundedToTwoSeconds(t *testing.T) {
	lf := &listingFake{block: 10 * time.Second}
	pf := &identityFake{profiles: []*identityv1.PublicProfile{{UserId: "seller-1", DisplayName: "seller.one"}}}
	r := newResolver(t, lf, pf)

	start := time.Now()
	got := r.ResolveSenderName(context.Background(), "seller-1", "seller-1")
	elapsed := time.Since(start)
	if got != "" {
		t.Fatalf("a timed-out lookup pair must yield empty, got %q", got)
	}
	if elapsed > 2500*time.Millisecond {
		t.Fatalf("resolver took %v, want within 2.5s", elapsed)
	}
	if elapsed < 1500*time.Millisecond {
		t.Fatalf("resolver gave up after %v, before the 2s bound", elapsed)
	}
}

func TestUpstreamPackageDeleted(t *testing.T) {
	if _, err := os.Stat("../upstream"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("internal/upstream must be deleted, stat err = %v", err)
	}
}
