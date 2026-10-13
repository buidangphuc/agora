// Package upstream dials the services team-order calls (team-domain,
// team-identity, team-promotion) with go-sdk's upstream.Dial and applies
// team-order's per-method service-principal scopes.
package upstream

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc"

	sdkupstream "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/upstream"

	identityv1 "github.com/buidangphuc/team-order/generated/platform/identity/v1"
	listingv1 "github.com/buidangphuc/team-order/generated/platform/listing/v1"
)

// Clients holds the connections to team-domain and team-identity.
type Clients struct {
	conns []*grpc.ClientConn
	// Listing wraps the listing client so ReserveStock/CommitReservation/ReleaseStock are always
	// marked as the service (team-order's own principal, scope listing.write only).
	Listing DomainClient
	Address identityv1.AddressServiceClient
}

// Service principal team-order presents to its upstreams. A call is made either
// as the forwarded end user (reads on behalf of a buyer) or as this service with
// exactly ONE scope chosen per call (least privilege): stock mutations carry
// only listing.write (team-domain), voucher saga RPCs only promotion.reserve
// (team-promotion). go-sdk's upstream.NewClient forwards the caller's principal and
// the request id and bounds each call; the per-call service principal comes from
// upstream.WithServicePrincipal, applied by the two client wrappers below.
const (
	servicePrincipalID = "service-team-order"
	stockServiceScope  = "listing.write"
	promotionScope     = "promotion.reserve"
)

// callTimeout bounds one unary upstream call. It matches the gateway's request
// budget, so the SDK bound never cuts a call shorter than the caller would wait.
const callTimeout = 5 * time.Second

// dial opens a lazily connecting go-sdk client to addr.
func dial(addr string) (*grpc.ClientConn, error) {
	return sdkupstream.NewClient(addr, sdkupstream.WithTimeout(callTimeout))
}

// asStock marks ctx so the call presents team-order's own service principal with
// only the stock scope (listing.write) instead of the end user's forwarded
// principal. The request id and trace headers are still forwarded.
func asStock(ctx context.Context) context.Context {
	return sdkupstream.WithServicePrincipal(ctx, servicePrincipalID, stockServiceScope)
}

// asPromotion is asStock for the voucher saga RPCs on team-promotion: same
// principal id, but the single scope promotion.reserve (never together with
// listing.write).
func asPromotion(ctx context.Context) context.Context {
	return sdkupstream.WithServicePrincipal(ctx, servicePrincipalID, promotionScope)
}

// Dial opens the lazy clients to team-domain and team-identity.
func Dial(domainAddr, identityAddr string) (*Clients, error) {
	c := &Clients{}

	domainConn, err := dial(domainAddr)
	if err != nil {
		return nil, fmt.Errorf("dial domain %s: %w", domainAddr, err)
	}
	c.conns = append(c.conns, domainConn)
	c.Listing = NewServiceStockClient(listingv1.NewListingServiceClient(domainConn))

	identityConn, err := dial(identityAddr)
	if err != nil {
		c.Close()
		return nil, fmt.Errorf("dial identity %s: %w", identityAddr, err)
	}
	c.conns = append(c.conns, identityConn)
	c.Address = identityv1.NewAddressServiceClient(identityConn)

	return c, nil
}

// Close closes every connection.
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
	// CommitReservation makes an active reservation permanent in team-domain so its
	// TTL sweep never restores the stock of a placed order.
	CommitReservation(ctx context.Context, req *listingv1.CommitReservationRequest, opts ...grpc.CallOption) (*listingv1.CommitReservationResponse, error)
}

// serviceStockClient marks exactly the stock-changing RPCs service-principal and leaves
// every other call (GetListing) untouched, so a buyer's read still forwards the
// buyer principal while stock changes carry team-order's own authority.
type serviceStockClient struct {
	DomainClient
}

// NewServiceStockClient wraps inner so ReserveStock, CommitReservation and ReleaseStock are sent as
// team-order's service principal, in a buyer's request and in background
// compensation/sweeping alike.
func NewServiceStockClient(inner DomainClient) DomainClient {
	if inner == nil {
		return nil
	}
	if already, ok := inner.(serviceStockClient); ok {
		return already
	}
	return serviceStockClient{DomainClient: inner}
}

func (c serviceStockClient) ReserveStock(ctx context.Context, req *listingv1.ReserveStockRequest, opts ...grpc.CallOption) (*listingv1.ReserveStockResponse, error) {
	return c.DomainClient.ReserveStock(asStock(ctx), req, opts...)
}

func (c serviceStockClient) ReleaseStock(ctx context.Context, req *listingv1.ReleaseStockRequest, opts ...grpc.CallOption) (*listingv1.ReleaseStockResponse, error) {
	return c.DomainClient.ReleaseStock(asStock(ctx), req, opts...)
}

func (c serviceStockClient) CommitReservation(ctx context.Context, req *listingv1.CommitReservationRequest, opts ...grpc.CallOption) (*listingv1.CommitReservationResponse, error) {
	return c.DomainClient.CommitReservation(asStock(ctx), req, opts...)
}
