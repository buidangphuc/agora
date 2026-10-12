// Package upstream dials the services team-order calls (team-domain,
// team-identity, team-promotion) with go-sdk's upstream.Dial and applies
// team-order's per-method service-principal scopes.
package upstream

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/auth"
	sdkupstream "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/upstream"

	identityv1 "github.com/buidangphuc/team-order/generated/platform/identity/v1"
	listingv1 "github.com/buidangphuc/team-order/generated/platform/listing/v1"
	promotionv1 "github.com/buidangphuc/team-order/generated/platform/promotion/v1"
)

// Clients holds the connections to team-domain and team-identity.
type Clients struct {
	conns []*grpc.ClientConn
	// Listing wraps the listing client so ReserveStock/CommitReservation/ReleaseStock are always
	// marked AsService (team-order's own principal, scope listing.write only).
	Listing DomainClient
	Address identityv1.AddressServiceClient
}

// Service principal team-order presents to its upstreams. A call is made either
// as the forwarded end user (reads on behalf of a buyer) or as this service with
// exactly ONE scope chosen per call (least privilege): stock mutations carry
// only listing.write (team-domain), voucher saga RPCs only promotion.reserve
// (team-promotion). Unmarked background calls get read-only defaults.
//
// go-sdk's upstream.Dial forwards the caller's principal and the request id and
// bounds each call. Its AsService option replaces the principal of EVERY call on
// a connection, which cannot express the per-method scopes below, so a small
// connection wrapper (scopedConn) puts the service principal on the call context
// and lets the SDK forward it.
const (
	servicePrincipalID   = "service-team-order"
	servicePrincipalType = "service"
	stockServiceScope    = "listing.write"
	promotionScope       = "promotion.reserve"
	defaultServiceScope  = "listing.read"
)

// callTimeout bounds one unary upstream call. It matches the gateway's request
// budget, so the SDK bound never cuts a call shorter than the caller would wait.
const callTimeout = 5 * time.Second

// methodScopes is the safety net for RPCs that must never carry the forwarded
// end-user principal even if a caller forgot to mark the context.
var methodScopes = map[string]string{
	listingv1.ListingService_ReserveStock_FullMethodName:         stockServiceScope,
	listingv1.ListingService_ReleaseStock_FullMethodName:         stockServiceScope,
	listingv1.ListingService_CommitReservation_FullMethodName:    stockServiceScope,
	promotionv1.VoucherService_ValidateAndReserve_FullMethodName: promotionScope,
	promotionv1.VoucherService_CommitReservation_FullMethodName:  promotionScope,
	promotionv1.VoucherService_ReleaseReservation_FullMethodName: promotionScope,
}

type asServiceKey struct{}

// AsService marks ctx so the client presents team-order's own service principal
// with only the stock scope (listing.write) instead of the end user's forwarded
// principal. The request id and trace headers are still forwarded.
func AsService(ctx context.Context) context.Context {
	return asServiceWithScope(ctx, stockServiceScope)
}

// asServiceWithPromotion is AsService for the voucher saga RPCs on team-promotion:
// same principal id/type, but the single scope promotion.reserve (never together
// with listing.write).
func asServiceWithPromotion(ctx context.Context) context.Context {
	return asServiceWithScope(ctx, promotionScope)
}

func asServiceWithScope(ctx context.Context, scope string) context.Context {
	return context.WithValue(ctx, asServiceKey{}, scope)
}

// IsService reports whether ctx was marked with AsService.
func IsService(ctx context.Context) bool {
	return serviceScope(ctx) != ""
}

// serviceScope returns the one scope a marked call presents ("" when unmarked).
func serviceScope(ctx context.Context) string {
	v, _ := ctx.Value(asServiceKey{}).(string)
	return v
}

// withCallPrincipal returns the context the SDK dialer forwards the principal of:
//   - a marked call, or a method in methodScopes, carries the service principal
//     with exactly that scope (never the buyer's);
//   - a call with no caller carries the read-only service identity;
//   - any other call keeps the caller's principal, scopes untouched (never widen a
//     buyer's scopes).
func withCallPrincipal(ctx context.Context, method string) context.Context {
	scope := serviceScope(ctx)
	if scope == "" {
		scope = methodScopes[method]
	}
	if scope == "" {
		if p, ok := auth.FromContext(ctx); ok && !p.Anonymous() {
			return ctx
		}
		scope = defaultServiceScope
	}
	return auth.WithPrincipal(ctx, auth.Principal{ID: servicePrincipalID, Type: servicePrincipalType, Scopes: []string{scope}})
}

// scopedConn is a grpc.ClientConnInterface that applies withCallPrincipal to every
// call before handing it to the SDK-dialed connection.
type scopedConn struct{ inner *grpc.ClientConn }

func (c scopedConn) Invoke(ctx context.Context, method string, args, reply any, opts ...grpc.CallOption) error {
	return c.inner.Invoke(withCallPrincipal(ctx, method), method, args, reply, opts...)
}

func (c scopedConn) NewStream(ctx context.Context, desc *grpc.StreamDesc, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	return c.inner.NewStream(withCallPrincipal(ctx, method), desc, method, opts...)
}

// dial opens a lazily connecting go-sdk client to addr behind scopedConn.
func dial(addr string) (*grpc.ClientConn, scopedConn, error) {
	conn, err := sdkupstream.Dial(addr, sdkupstream.WithTimeout(callTimeout))
	if err != nil {
		return nil, scopedConn{}, err
	}
	return conn, scopedConn{inner: conn}, nil
}

// Dial opens the lazy clients to team-domain and team-identity.
func Dial(domainAddr, identityAddr string) (*Clients, error) {
	c := &Clients{}

	domainConn, domain, err := dial(domainAddr)
	if err != nil {
		return nil, fmt.Errorf("dial domain %s: %w", domainAddr, err)
	}
	c.conns = append(c.conns, domainConn)
	c.Listing = NewServiceStockClient(listingv1.NewListingServiceClient(domain))

	identityConn, identity, err := dial(identityAddr)
	if err != nil {
		c.Close()
		return nil, fmt.Errorf("dial identity %s: %w", identityAddr, err)
	}
	c.conns = append(c.conns, identityConn)
	c.Address = identityv1.NewAddressServiceClient(identity)

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

// serviceStockClient marks exactly the stock-changing RPCs AsService and leaves
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
	return c.DomainClient.ReserveStock(AsService(ctx), req, opts...)
}

func (c serviceStockClient) ReleaseStock(ctx context.Context, req *listingv1.ReleaseStockRequest, opts ...grpc.CallOption) (*listingv1.ReleaseStockResponse, error) {
	return c.DomainClient.ReleaseStock(AsService(ctx), req, opts...)
}

func (c serviceStockClient) CommitReservation(ctx context.Context, req *listingv1.CommitReservationRequest, opts ...grpc.CallOption) (*listingv1.CommitReservationResponse, error) {
	return c.DomainClient.CommitReservation(AsService(ctx), req, opts...)
}
