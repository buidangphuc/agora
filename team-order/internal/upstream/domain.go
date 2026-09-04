package upstream

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	identityv1 "github.com/buidangphuc/team-order/generated/platform/identity/v1"
	listingv1 "github.com/buidangphuc/team-order/generated/platform/listing/v1"
)

type Clients struct {
	conns   []*grpc.ClientConn
	Listing listingv1.ListingServiceClient
	Address identityv1.AddressServiceClient
}

func forwardMetadataInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		var outMD metadata.MD
		if inMD, ok := metadata.FromIncomingContext(ctx); ok {
			outMD = inMD.Copy()
		} else {
			outMD = metadata.MD{}
		}

		if len(outMD.Get("x-principal-id")) == 0 {
			outMD.Set("x-principal-id", "service-team-order")
			outMD.Set("x-principal-type", "service")
			outMD.Set("x-principal-scopes", "listing.read,listing.write,identity.read,identity.write")
		} else {
			// Ensure listing.read is in the scopes for downstream listing requests
			scopes := outMD.Get("x-principal-scopes")
			if len(scopes) > 0 {
				outMD.Set("x-principal-scopes", scopes[0]+",listing.read,identity.read")
			}
		}

		ctx = metadata.NewOutgoingContext(ctx, outMD)
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

func Dial(domainAddr, identityAddr string) (*Clients, error) {
	insec := grpc.WithTransportCredentials(insecure.NewCredentials())
	interceptor := grpc.WithUnaryInterceptor(forwardMetadataInterceptor())
	c := &Clients{}

	domainConn, err := grpc.NewClient(domainAddr, insec, interceptor)
	if err != nil {
		return nil, fmt.Errorf("dial domain %s: %w", domainAddr, err)
	}
	c.conns = append(c.conns, domainConn)
	c.Listing = listingv1.NewListingServiceClient(domainConn)

	identityConn, err := grpc.NewClient(identityAddr, insec, interceptor)
	if err != nil {
		c.Close()
		return nil, fmt.Errorf("dial identity %s: %w", identityAddr, err)
	}
	c.conns = append(c.conns, identityConn)
	c.Address = identityv1.NewAddressServiceClient(identityConn)

	return c, nil
}

func (c *Clients) Close() {
	for _, conn := range c.conns {
		if conn != nil {
			_ = conn.Close()
		}
	}
}

// DomainClient interface for testability
type DomainClient interface {
	GetListing(ctx context.Context, req *listingv1.GetListingRequest, opts ...grpc.CallOption) (*listingv1.GetListingResponse, error)
	ReserveStock(ctx context.Context, req *listingv1.ReserveStockRequest, opts ...grpc.CallOption) (*listingv1.ReserveStockResponse, error)
	ReleaseStock(ctx context.Context, req *listingv1.ReleaseStockRequest, opts ...grpc.CallOption) (*listingv1.ReleaseStockResponse, error)
}
