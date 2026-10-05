package upstream

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	identityv1 "github.com/buidangphuc/team-notification/generated/platform/identity/v1"
	listingv1 "github.com/buidangphuc/team-notification/generated/platform/listing/v1"
)

type fakeStorefronts struct {
	shops []*listingv1.ShopSummary
	err   error
	calls int
	md    metadata.MD
	req   *listingv1.BatchGetStorefrontsRequest
}

func (f *fakeStorefronts) BatchGetStorefronts(ctx context.Context, in *listingv1.BatchGetStorefrontsRequest, _ ...grpc.CallOption) (*listingv1.BatchGetStorefrontsResponse, error) {
	f.calls++
	f.req = in
	f.md, _ = metadata.FromOutgoingContext(ctx)
	if f.err != nil {
		return nil, f.err
	}
	return &listingv1.BatchGetStorefrontsResponse{Shops: f.shops}, nil
}

type fakeProfiles struct {
	profiles []*identityv1.PublicProfile
	err      error
	calls    int
	md       metadata.MD
}

func (f *fakeProfiles) GetPublicProfiles(ctx context.Context, _ *identityv1.GetPublicProfilesRequest, _ ...grpc.CallOption) (*identityv1.GetPublicProfilesResponse, error) {
	f.calls++
	f.md, _ = metadata.FromOutgoingContext(ctx)
	if f.err != nil {
		return nil, f.err
	}
	return &identityv1.GetPublicProfilesResponse{Profiles: f.profiles}, nil
}

func quiet() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestResolve_SellerGetsShopName(t *testing.T) {
	sf := &fakeStorefronts{shops: []*listingv1.ShopSummary{{SellerId: "seller-1", DisplayName: "Nhà Sách An Nhiên"}}}
	pf := &fakeProfiles{}
	r := NewNameResolver(sf, pf, quiet())

	if got := r.ResolveSenderName(context.Background(), "seller-1", "seller-1"); got != "Nhà Sách An Nhiên" {
		t.Fatalf("name = %q", got)
	}
	if pf.calls != 0 {
		t.Fatal("identity must not be called when the shop name resolves")
	}
	// Service-principal convention: the same metadata team-order's upstream client sends.
	if sf.md.Get("x-principal-id")[0] != "service-team-notification" ||
		sf.md.Get("x-principal-type")[0] != "service" ||
		sf.md.Get("x-principal-scopes")[0] != "listing.read,identity.read" {
		t.Fatalf("principal metadata = %v", sf.md)
	}
	if len(sf.req.GetSellerIds()) != 1 || sf.req.GetSellerIds()[0] != "seller-1" {
		t.Fatalf("storefront request = %v", sf.req)
	}
}

func TestResolve_BuyerGetsUserDisplayName(t *testing.T) {
	sf := &fakeStorefronts{shops: []*listingv1.ShopSummary{{SellerId: "buyer-1", DisplayName: "A shop the buyer happens to own"}}}
	pf := &fakeProfiles{profiles: []*identityv1.PublicProfile{{UserId: "buyer-1", DisplayName: "an.nguyen"}}}
	r := NewNameResolver(sf, pf, quiet())

	// buyer-1 is not the thread's seller: the shop lookup is skipped entirely.
	if got := r.ResolveSenderName(context.Background(), "buyer-1", "seller-1"); got != "an.nguyen" {
		t.Fatalf("name = %q", got)
	}
	if sf.calls != 0 {
		t.Fatal("a non-seller sender must not hit team-domain")
	}
	if pf.md.Get("x-principal-type")[0] != "service" || pf.md.Get("x-principal-scopes")[0] != "listing.read,identity.read" {
		t.Fatalf("principal metadata = %v", pf.md)
	}
}

func TestResolve_SellerWithoutShopFallsBackToProfile(t *testing.T) {
	sf := &fakeStorefronts{} // no storefront
	pf := &fakeProfiles{profiles: []*identityv1.PublicProfile{{UserId: "seller-1", DisplayName: "seller.one"}}}
	if got := NewNameResolver(sf, pf, quiet()).ResolveSenderName(context.Background(), "seller-1", "seller-1"); got != "seller.one" {
		t.Fatalf("name = %q", got)
	}
}

func TestResolve_LookupFailureYieldsEmpty(t *testing.T) {
	boom := errors.New("unavailable")
	r := NewNameResolver(&fakeStorefronts{err: boom}, &fakeProfiles{err: boom}, quiet())
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

func TestResolve_NilClientsYieldEmpty(t *testing.T) {
	if got := NewNameResolver(nil, nil, quiet()).ResolveSenderName(context.Background(), "u", "u"); got != "" {
		t.Fatalf("got %q", got)
	}
}
