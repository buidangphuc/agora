package upstream

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"

	listingv1 "github.com/buidangphuc/team-promotion/generated/platform/listing/v1"
	"github.com/buidangphuc/team-promotion/internal/listing"
)

type recordingListings struct {
	listingv1.UnimplementedListingServiceServer
	mu    sync.Mutex
	mds   []metadata.MD
	block bool
	owner string
}

func (r *recordingListings) GetListing(ctx context.Context, req *listingv1.GetListingRequest) (*listingv1.GetListingResponse, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	r.mu.Lock()
	r.mds = append(r.mds, md)
	r.mu.Unlock()
	if r.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if req.GetId() == "ghost" {
		return nil, status.Error(codes.NotFound, "not_found")
	}
	return &listingv1.GetListingResponse{Listing: &listingv1.Listing{Id: req.GetId(), SellerId: r.owner}}, nil
}

func startListings(t *testing.T, svc *recordingListings) string {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := grpc.NewServer()
	listingv1.RegisterListingServiceServer(srv, svc)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	return lis.Addr().String()
}

func newListingClient(t *testing.T, addr string) *ListingClient {
	t.Helper()
	c, err := NewListingClient(addr)
	if err != nil || c == nil {
		t.Fatalf("NewListingClient: %v, %v", c, err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func sellerCtx(base context.Context, scopes ...string) context.Context {
	return auth.WithPrincipal(base, auth.Principal{ID: "seller-1", Type: auth.TypeUser, Scopes: scopes})
}

func lookupScopes(t *testing.T, ctxScopes []string) metadata.MD {
	t.Helper()
	base := t.Context()
	rec := &recordingListings{owner: "seller-1"}
	c := newListingClient(t, startListings(t, rec))
	scopes := make([]string, len(ctxScopes), len(ctxScopes)+4)
	copy(scopes, ctxScopes)
	ctx := sellerCtx(base, scopes...)
	if _, err := c.GetListing(ctx, &listingv1.GetListingRequest{Id: "mine"}); err != nil {
		t.Fatalf("GetListing: %v", err)
	}
	if p, _ := auth.FromContext(ctx); len(p.Scopes) != len(ctxScopes) {
		t.Errorf("caller principal scopes mutated: %v", p.Scopes)
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.mds) != 1 {
		t.Fatalf("want 1 GetListing call, got %d", len(rec.mds))
	}
	return rec.mds[0]
}

// Scenario: Listing lookup runs as the caller with listing.read
func TestListingLookupForwardsCallerWithListingRead(t *testing.T) {
	md := lookupScopes(t, []string{"listing.write"})
	for k, v := range map[string]string{
		"x-principal-id":     "seller-1",
		"x-principal-type":   "user",
		"x-principal-scopes": "listing.write,listing.read",
	} {
		if vals := md.Get(k); len(vals) != 1 || vals[0] != v {
			t.Errorf("%s = %v, want [%s]", k, vals, v)
		}
	}
}

// Scenario: Listing lookup runs as the caller with listing.read (kept domain_test coverage)
func TestForwardPrincipalKeepsCallerAndAddsListingRead(t *testing.T) {
	md := lookupScopes(t, []string{"listing.write"})
	if v := md.Get("x-principal-id"); len(v) != 1 || v[0] != "seller-1" {
		t.Fatalf("principal id = %v, want caller", v)
	}
	if v := md.Get("x-principal-scopes"); len(v) != 1 || !hasScope(v[0], "listing.read") || !hasScope(v[0], "listing.write") {
		t.Fatalf("scopes = %v", v)
	}
}

// Scenario: Listing lookup runs as the caller with listing.read (already held, not duplicated)
func TestListingLookupDoesNotDuplicateListingRead(t *testing.T) {
	md := lookupScopes(t, []string{"listing.read", "listing.write"})
	if v := md.Get("x-principal-scopes"); len(v) != 1 || v[0] != "listing.read,listing.write" {
		t.Fatalf("scopes = %v, want [listing.read,listing.write]", v)
	}
}

func hasScope(csv, want string) bool {
	start := 0
	for i := 0; i <= len(csv); i++ {
		if i == len(csv) || csv[i] == ',' {
			if csv[start:i] == want {
				return true
			}
			start = i + 1
		}
	}
	return false
}

// Scenario: A slow listing lookup is unavailable
func TestListingLookupTimeoutIsUnavailable(t *testing.T) {
	base := t.Context()
	c := newListingClient(t, startListings(t, &recordingListings{block: true}))
	start := time.Now()
	err := listing.Owner(sellerCtx(base, "listing.write"), c, discardLogger(), "mine", "seller-1")
	if err != listing.ErrLookupFailed {
		t.Fatalf("err = %v, want listing.ErrLookupFailed", err)
	}
	if el := time.Since(start); el > 2500*time.Millisecond {
		t.Fatalf("lookup took %v, want within 2.5s", el)
	}
}

// Scenario: Listing lookup runs as the caller with listing.read (owner outcomes over the real client)
func TestListingLookupOwnerOutcomes(t *testing.T) {
	base := t.Context()
	c := newListingClient(t, startListings(t, &recordingListings{owner: "seller-1"}))
	ctx := sellerCtx(base, "listing.write")
	if err := listing.Owner(ctx, c, discardLogger(), "mine", "seller-1"); err != nil {
		t.Fatalf("own listing: %v", err)
	}
	if err := listing.Owner(ctx, c, discardLogger(), "mine", "seller-2"); err != listing.ErrNotOwner {
		t.Fatalf("foreign listing: err = %v, want ErrNotOwner", err)
	}
	if err := listing.Owner(ctx, c, discardLogger(), "ghost", "seller-1"); err != listing.ErrNotFound {
		t.Fatalf("missing listing: err = %v, want ErrNotFound", err)
	}
}
