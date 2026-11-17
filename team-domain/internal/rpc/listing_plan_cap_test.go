package rpc_test

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"testing"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	sdkconfig "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/grpcx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/healthx"
	listingv1 "github.com/buidangphuc/team-domain/generated/platform/listing/v1"
	"github.com/buidangphuc/team-domain/internal/listing"
	"github.com/buidangphuc/team-domain/internal/objects"
	"github.com/buidangphuc/team-domain/internal/rpc"
	"github.com/buidangphuc/team-domain/internal/store"
	"github.com/buidangphuc/team-domain/internal/store/storetest"
)

const capScopes = "listing.read,listing.write"

type capFakeEnt struct {
	mu      sync.Mutex
	ent     listing.Entitlements
	err     error
	calls   int
	sellers []string
}

func (f *capFakeEnt) Get(_ context.Context, sellerID string) (listing.Entitlements, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.sellers = append(f.sellers, sellerID)
	return f.ent, f.err
}

func (f *capFakeEnt) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

type capRig struct {
	client listingv1.ListingServiceClient
	repo   *storetest.InMemoryListingRepository
	txw    *storetest.InMemoryTxWriter
	ent    *capFakeEnt
}

func startCapRig(t *testing.T, owned map[string]int, maxListings int64) *capRig {
	t.Helper()
	seed := []store.Listing{{ID: "sentinel", Title: "s", Currency: "VND", Status: "published", SellerID: "nobody"}}
	for seller, n := range owned {
		for i := 0; i < n; i++ {
			seed = append(seed, store.Listing{
				ID: fmt.Sprintf("%s-%d", seller, i), Title: "t", Currency: "VND", Status: "draft", SellerID: seller,
			})
		}
	}
	repo := storetest.NewInMemoryListingRepository(seed...)
	txw := storetest.NewInMemoryTxWriter(repo)
	ent := &capFakeEnt{ent: listing.Entitlements{MaxListings: maxListings}}
	svc := listing.NewListingServiceWithOutbox(repo, txw).WithEntitlements(ent)
	h := rpc.NewListingHandler(svc, storetest.NewInMemoryCategoryRepository(), objects.NewS3Storage(testStorage()))

	logger := slog.New(slog.DiscardHandler)
	health, err := healthx.New(healthx.DefaultInterval, logger)
	if err != nil {
		t.Fatalf("health: %v", err)
	}
	srv := grpcx.NewServer(grpcx.ServerOptions{Config: sdkconfig.GRPC{}, Logger: logger, Health: health})
	listingv1.RegisterListingServiceServer(srv, h)
	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() { _ = srv.Serve(lis) }()
	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close(); srv.Stop() })
	return &capRig{client: listingv1.NewListingServiceClient(conn), repo: repo, txw: txw, ent: ent}
}

func (r *capRig) owned(t *testing.T, seller string) int64 {
	t.Helper()
	n, err := r.repo.ListByOwner(t.Context(), seller, store.ListingFilter{}, "", 100)
	if err != nil {
		t.Fatal(err)
	}
	return n.Total
}

func capDraft() *listingv1.Listing {
	return &listingv1.Listing{
		Title: "new", Currency: "VND", Price: 10,
		Status: listingv1.ListingStatus_LISTING_STATUS_DRAFT,
	}
}

func errorReason(t *testing.T, err error) string {
	t.Helper()
	for _, d := range status.Convert(err).Details() {
		if info, ok := d.(*errdetails.ErrorInfo); ok {
			return info.GetReason()
		}
	}
	return ""
}

func TestCreateRejectedAtPlanCapOverRPC(t *testing.T) {
	rig := startCapRig(t, map[string]int{"s1": 10}, 10)
	ctx, cancel := principalCtxAs(t, "s1", "user", capScopes)
	defer cancel()

	_, err := rig.client.CreateListing(ctx, &listingv1.CreateListingRequest{Listing: capDraft()})
	if status.Code(err) != codes.ResourceExhausted {
		t.Fatalf("code = %v (%v), want ResourceExhausted", status.Code(err), err)
	}
	if got := errorReason(t, err); got != "PLAN_LISTING_LIMIT" {
		t.Fatalf("ErrorInfo.Reason = %q, want PLAN_LISTING_LIMIT", got)
	}
	if got := rig.owned(t, "s1"); got != 10 {
		t.Fatalf("owned = %d, want 10", got)
	}
}

func TestCreateFailsClosedWhenPromotionDownOverRPC(t *testing.T) {
	rig := startCapRig(t, map[string]int{"s1": 0}, 10)
	rig.ent.err = errors.New("connection refused")
	ctx, cancel := principalCtxAs(t, "s1", "user", capScopes)
	defer cancel()

	_, err := rig.client.CreateListing(ctx, &listingv1.CreateListingRequest{Listing: capDraft()})
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("code = %v (%v), want Unavailable", status.Code(err), err)
	}
	if got := rig.owned(t, "s1"); got != 0 {
		t.Fatalf("owned = %d, want 0", got)
	}
	if len(rig.txw.Rows()) != 0 {
		t.Fatalf("outbox rows = %d, want 0", len(rig.txw.Rows()))
	}
}

func TestCreateCapUsesPrincipalNotBodySeller(t *testing.T) {
	rig := startCapRig(t, map[string]int{"sA": 10, "sB": 0}, 10)
	ctx, cancel := principalCtxAs(t, "sB", "user", capScopes)
	defer cancel()

	body := capDraft()
	body.SellerId = "sA"
	resp, err := rig.client.CreateListing(ctx, &listingv1.CreateListingRequest{Listing: body})
	if err != nil {
		t.Fatalf("CreateListing as sB: %v", err)
	}
	if resp.GetListing().GetSellerId() != "sB" {
		t.Fatalf("owner = %q, want sB", resp.GetListing().GetSellerId())
	}
	if rig.owned(t, "sA") != 10 || rig.owned(t, "sB") != 1 {
		t.Fatalf("owned sA=%d sB=%d, want 10 and 1", rig.owned(t, "sA"), rig.owned(t, "sB"))
	}
	rig.ent.mu.Lock()
	defer rig.ent.mu.Unlock()
	if len(rig.ent.sellers) != 1 || rig.ent.sellers[0] != "sB" {
		t.Fatalf("entitlements asked for %v, want [sB]", rig.ent.sellers)
	}
}

func TestCreateWithoutScopeSkipsEntitlementLookup(t *testing.T) {
	rig := startCapRig(t, map[string]int{"s1": 0}, 10)
	ctx, cancel := principalCtxAs(t, "s1", "user", "listing.read")
	defer cancel()

	_, err := rig.client.CreateListing(ctx, &listingv1.CreateListingRequest{Listing: capDraft()})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("code = %v (%v), want PermissionDenied", status.Code(err), err)
	}
	if rig.ent.count() != 0 {
		t.Fatalf("entitlement port called %d times, want 0", rig.ent.count())
	}
}

func TestNonCreatePathsIgnoreEntitlements(t *testing.T) {
	rig := startCapRig(t, map[string]int{"s1": 12}, 10)
	rig.ent.err = errors.New("connection refused")
	ctx, cancel := principalCtxAs(t, "s1", "user", capScopes)
	defer cancel()

	upd := capDraft()
	upd.Id = "s1-0"
	upd.Title = "edited"
	if _, err := rig.client.UpdateListing(ctx, &listingv1.UpdateListingRequest{Listing: upd}); err != nil {
		t.Fatalf("UpdateListing: %v", err)
	}
	if _, err := rig.client.DeleteListing(ctx, &listingv1.DeleteListingRequest{Id: "s1-1"}); err != nil {
		t.Fatalf("DeleteListing: %v", err)
	}
	res, err := rig.client.ListMyListings(ctx, &listingv1.ListMyListingsRequest{})
	if err != nil {
		t.Fatalf("ListMyListings: %v", err)
	}
	if len(res.GetListings()) != 11 {
		t.Fatalf("listings = %d, want 11", len(res.GetListings()))
	}
	if rig.ent.count() != 0 {
		t.Fatalf("entitlement port called %d times, want 0", rig.ent.count())
	}
}

func TestOverCapListingsStillOwnerOnly(t *testing.T) {
	rig := startCapRig(t, map[string]int{"sA": 14, "sB": 0}, 10)
	ctx, cancel := principalCtxAs(t, "sB", "user", capScopes)
	defer cancel()

	upd := capDraft()
	upd.Id = "sA-0"
	upd.Title = "hijack"
	_, err := rig.client.UpdateListing(ctx, &listingv1.UpdateListingRequest{Listing: upd})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("code = %v (%v), want PermissionDenied", status.Code(err), err)
	}
	got, err := rig.repo.Get(t.Context(), "sA-0")
	if err != nil || got.Title != "t" {
		t.Fatalf("listing changed: %+v err=%v", got, err)
	}
}
