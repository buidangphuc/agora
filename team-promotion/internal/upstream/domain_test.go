package upstream_test

import (
	"context"
	"net"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/auth"
	sdkupstream "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/upstream"

	listingv1 "github.com/buidangphuc/team-promotion/generated/platform/listing/v1"
	"github.com/buidangphuc/team-promotion/internal/upstream"
)

// fakeDomain is a team-domain ListingService that records the metadata it sees,
// and can hang until the call is cancelled.
type fakeDomain struct {
	listingv1.UnimplementedListingServiceServer
	hang bool

	mu sync.Mutex
	md metadata.MD
}

func (f *fakeDomain) GetListing(ctx context.Context, _ *listingv1.GetListingRequest) (*listingv1.GetListingResponse, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	f.mu.Lock()
	f.md = md
	f.mu.Unlock()
	if f.hang {
		<-ctx.Done()
		return nil, status.FromContextError(ctx.Err()).Err()
	}
	return &listingv1.GetListingResponse{}, nil
}

func serve(t *testing.T, f *fakeDomain) string {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := grpc.NewServer()
	listingv1.RegisterListingServiceServer(srv, f)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	return lis.Addr().String()
}

func TestForwardPrincipalKeepsCallerAndAddsListingRead(t *testing.T) {
	f := &fakeDomain{}
	d, err := upstream.Dial(serve(t, f), 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d.Close(context.Background()) }()

	in := auth.WithPrincipal(context.Background(), auth.Principal{ID: "seller-1", Type: "user", Scopes: []string{"listing.write"}})
	if _, err := d.GetListing(in, &listingv1.GetListingRequest{Id: "l1"}); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	got := f.md
	f.mu.Unlock()
	if v := got.Get("x-principal-id"); len(v) != 1 || v[0] != "seller-1" {
		t.Fatalf("principal id = %v, want caller", v)
	}
	if v := got.Get("x-principal-type"); len(v) != 1 || v[0] != "user" {
		t.Fatalf("principal type = %v, want caller's", v)
	}
	if v := got.Get("x-principal-scopes"); len(v) != 1 || !slices.Contains(strings.Split(v[0], ","), "listing.read") || !slices.Contains(strings.Split(v[0], ","), "listing.write") {
		t.Fatalf("scopes = %v", v)
	}
}

func TestWithTimeoutBoundsHangingLookup(t *testing.T) {
	d, err := upstream.Dial(serve(t, &fakeDomain{hang: true}), 50*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d.Close(context.Background()) }()

	start := time.Now()
	_, err = d.GetListing(context.Background(), &listingv1.GetListingRequest{Id: "l1"})
	if status.Code(err) != codes.DeadlineExceeded {
		t.Fatalf("err = %v, want DeadlineExceeded", err)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatalf("lookup not bounded: %v", time.Since(start))
	}
}

func TestBoundedCtxRespectsInboundDeadline(t *testing.T) {
	parent, cancel := context.WithTimeout(context.Background(), 600*time.Millisecond)
	defer cancel()
	ctx, c := sdkupstream.Bounded(parent, 10*time.Second)
	defer c()
	dl, _ := ctx.Deadline()
	if rem := time.Until(dl); rem > 200*time.Millisecond {
		t.Fatalf("budget %v exceeds inbound deadline minus margin", rem)
	}

	expired, c2 := sdkupstream.Bounded(func() context.Context {
		p, cc := context.WithTimeout(context.Background(), 100*time.Millisecond)
		_ = cc
		return p
	}(), time.Second)
	defer c2()
	if expired.Err() == nil {
		select {
		case <-expired.Done():
		default:
			t.Fatal("context inside the margin must be already expired")
		}
	}
}

func TestWithTimeoutNilStaysNil(t *testing.T) {
	if upstream.Listings(nil) != nil {
		t.Fatal("nil getter must stay nil so callers fail closed")
	}
}
