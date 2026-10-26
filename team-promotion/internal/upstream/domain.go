package upstream

import (
	"context"
	"fmt"
	"slices"

	"google.golang.org/grpc"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	sdkupstream "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/upstream"

	listingv1 "github.com/buidangphuc/team-promotion/generated/platform/listing/v1"
)

const listingReadScope = "listing.read"

type ListingClient struct {
	conn *grpc.ClientConn
	svc  listingv1.ListingServiceClient
}

func NewListingClient(addr string) (*ListingClient, error) {
	conn, err := sdkupstream.NewClient(addr, sdkupstream.WithDialOptions(
		grpc.WithDefaultCallOptions(grpc.WaitForReady(true)),
	))
	if err != nil {
		return nil, fmt.Errorf("dial team-domain at %s: %w", addr, err)
	}
	return &ListingClient{conn: conn, svc: listingv1.NewListingServiceClient(conn)}, nil
}

func (c *ListingClient) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

func (c *ListingClient) GetListing(ctx context.Context, req *listingv1.GetListingRequest, opts ...grpc.CallOption) (*listingv1.GetListingResponse, error) {
	if p, ok := auth.FromContext(ctx); ok && !slices.Contains(p.Scopes, listingReadScope) {
		p.Scopes = append(slices.Clone(p.Scopes), listingReadScope)
		ctx = auth.WithPrincipal(ctx, p)
	}
	return c.svc.GetListing(ctx, req, opts...)
}
