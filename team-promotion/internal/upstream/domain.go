// Package upstream holds team-promotion's outbound gRPC clients.
package upstream

import (
	"context"
	"slices"
	"time"

	"google.golang.org/grpc"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/auth"
	sdkupstream "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/upstream"

	listingv1 "github.com/buidangphuc/team-promotion/generated/platform/listing/v1"
)

// scopeListingRead is what team-domain's GetListing requires; a seller role
// already holds it.
const scopeListingRead = "listing.read"

// ListingGetter is the slice of team-domain's ListingService that team-promotion
// needs: reading a listing to learn who owns it.
type ListingGetter interface {
	GetListing(ctx context.Context, req *listingv1.GetListingRequest, opts ...grpc.CallOption) (*listingv1.GetListingResponse, error)
}

// Domain is a dialed connection to team-domain.
type Domain struct {
	conn    *grpc.ClientConn
	listing listingv1.ListingServiceClient
}

// Dial connects to team-domain at addr (the SDK dialer: lazy, request id and
// the caller's principal forwarded). Every call is bounded by timeout and by
// the inbound request deadline; a non-positive timeout keeps the SDK default.
func Dial(addr string, timeout time.Duration) (*Domain, error) {
	var opts []sdkupstream.Option
	if timeout > 0 {
		opts = append(opts, sdkupstream.WithTimeout(timeout))
	}
	conn, err := sdkupstream.Dial(addr, opts...)
	if err != nil {
		return nil, err
	}
	return &Domain{conn: conn, listing: listingv1.NewListingServiceClient(conn)}, nil
}

// Close releases the connection.
func (d *Domain) Close(context.Context) error { return d.conn.Close() }

// GetListing reads a listing as the caller, with listing.read added to the
// caller's scopes. After team-domain restarts with a new address the first call
// would otherwise fail fast (connection refused on the old IP) while the
// resolver catches up, so it waits for the reconnect; the call timeout bounds it.
func (d *Domain) GetListing(ctx context.Context, req *listingv1.GetListingRequest, opts ...grpc.CallOption) (*listingv1.GetListingResponse, error) {
	opts = append([]grpc.CallOption{grpc.WaitForReady(true)}, opts...)
	return d.listing.GetListing(withListingRead(ctx), req, opts...)
}

// Listings returns d as a ListingGetter, or a nil interface when d is nil, so
// callers fail closed instead of calling through a typed nil.
func Listings(d *Domain) ListingGetter {
	if d == nil {
		return nil
	}
	return d
}

// withListingRead adds listing.read to the caller's scopes when the caller
// carries scopes at all, so the SDK dialer forwards the caller's own principal
// with that one extra scope.
func withListingRead(ctx context.Context) context.Context {
	p, ok := auth.FromContext(ctx)
	if !ok || len(p.Scopes) == 0 || p.HasScope(scopeListingRead) {
		return ctx
	}
	p.Scopes = append(slices.Clone(p.Scopes), scopeListingRead)
	return auth.WithPrincipal(ctx, p)
}
