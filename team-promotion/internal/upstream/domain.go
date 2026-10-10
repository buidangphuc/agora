// Package upstream holds team-promotion's outbound gRPC clients.
package upstream

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	listingv1 "github.com/buidangphuc/team-promotion/generated/platform/listing/v1"
)

// ListingGetter is the slice of team-domain's ListingService that team-promotion
// needs: reading a listing to learn who owns it.
type ListingGetter interface {
	GetListing(ctx context.Context, req *listingv1.GetListingRequest, opts ...grpc.CallOption) (*listingv1.GetListingResponse, error)
}

// Domain is a dialed connection to team-domain.
type Domain struct {
	conn    *grpc.ClientConn
	Listing listingv1.ListingServiceClient
}

// Dial connects to team-domain at addr. Calls forward the caller's principal.
func Dial(addr string) (*Domain, error) {
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(forwardPrincipalInterceptor()),
		// After team-domain restarts with a new address, the first call would
		// otherwise fail fast (connection refused on the old IP) while the
		// resolver catches up. Wait for the reconnect instead; every call already
		// carries the UPSTREAM_CALL_TIMEOUT_SECONDS deadline, which bounds it.
		grpc.WithDefaultCallOptions(grpc.WaitForReady(true)),
	)
	if err != nil {
		return nil, fmt.Errorf("dial domain %s: %w", addr, err)
	}
	return &Domain{conn: conn, Listing: listingv1.NewListingServiceClient(conn)}, nil
}

// Close releases the connection.
func (d *Domain) Close() {
	if d != nil && d.conn != nil {
		_ = d.conn.Close()
	}
}

// forwardPrincipalInterceptor forwards the caller's incoming principal metadata
// (the ownership lookup runs "as the caller") and makes sure listing.read is
// present: team-domain's GetListing requires it and a seller role already holds it.
func forwardPrincipalInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		outMD := metadata.MD{}
		if inMD, ok := metadata.FromIncomingContext(ctx); ok {
			outMD = inMD.Copy()
		}
		scopes := outMD.Get("x-principal-scopes")
		if len(scopes) > 0 && !hasScope(scopes[0], "listing.read") {
			outMD.Set("x-principal-scopes", scopes[0]+",listing.read")
		}
		return invoker(metadata.NewOutgoingContext(ctx, outMD), method, req, reply, cc, opts...)
	}
}

func hasScope(csv, want string) bool {
	for _, s := range strings.Split(csv, ",") {
		if strings.TrimSpace(s) == want {
			return true
		}
	}
	return false
}
