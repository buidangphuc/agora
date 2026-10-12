package upstream

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/reqctx"

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

func quiet() *slog.Logger { return slog.New(slog.DiscardHandler) }

// The fakes served over a real gRPC listener, so the principal the SDK upstream
// connection puts on the wire is observable. The recorded md is what the server
// received (incoming metadata).
type listingServer struct {
	listingv1.UnimplementedListingServiceServer
	f *fakeStorefronts
}

func (s listingServer) BatchGetStorefronts(ctx context.Context, in *listingv1.BatchGetStorefrontsRequest) (*listingv1.BatchGetStorefrontsResponse, error) {
	resp, err := s.f.BatchGetStorefronts(ctx, in)
	s.f.md, _ = metadata.FromIncomingContext(ctx)
	return resp, err
}

type profileServer struct {
	identityv1.UnimplementedPublicProfileServiceServer
	f *fakeProfiles
}

func (s profileServer) GetPublicProfiles(ctx context.Context, in *identityv1.GetPublicProfilesRequest) (*identityv1.GetPublicProfilesResponse, error) {
	resp, err := s.f.GetPublicProfiles(ctx, in)
	s.f.md, _ = metadata.FromIncomingContext(ctx)
	return resp, err
}

// serve starts sf and pf behind one gRPC server and returns Clients dialled to it
// with Dial, the production path.
func serve(t *testing.T, sf *fakeStorefronts, pf *fakeProfiles) *Clients {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := grpc.NewServer()
	listingv1.RegisterListingServiceServer(srv, listingServer{f: sf})
	identityv1.RegisterPublicProfileServiceServer(srv, profileServer{f: pf})
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	c, err := Dial(lis.Addr().String(), lis.Addr().String(), quiet())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	return c
}

func TestResolve_SellerGetsShopName(t *testing.T) {
	sf := &fakeStorefronts{shops: []*listingv1.ShopSummary{{SellerId: "seller-1", DisplayName: "Nhà Sách An Nhiên"}}}
	pf := &fakeProfiles{}
	r := serve(t, sf, pf).Resolver

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
	r := serve(t, sf, pf).Resolver

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

// The SDK connection carries the request id of the calling context to the
// upstream, so a lookup can be traced back to the event that caused it.
func TestResolve_ForwardsRequestID(t *testing.T) {
	sf := &fakeStorefronts{shops: []*listingv1.ShopSummary{{SellerId: "seller-1", DisplayName: "Shop"}}}
	r := serve(t, sf, &fakeProfiles{}).Resolver

	ctx := reqctx.WithRequestID(context.Background(), "req-42")
	if got := r.ResolveSenderName(ctx, "seller-1", "seller-1"); got != "Shop" {
		t.Fatalf("name = %q", got)
	}
	if ids := sf.md.Get("x-request-id"); len(ids) != 1 || ids[0] != "req-42" {
		t.Fatalf("request id metadata = %v", sf.md)
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
