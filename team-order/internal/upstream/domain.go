package upstream

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	identityv1 "github.com/buidangphuc/team-order/generated/platform/identity/v1"
	listingv1 "github.com/buidangphuc/team-order/generated/platform/listing/v1"
	promotionv1 "github.com/buidangphuc/team-order/generated/platform/promotion/v1"
)

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
const (
	servicePrincipalID   = "service-team-order"
	servicePrincipalType = "service"
	stockServiceScope    = "listing.write"
	promotionScope       = "promotion.reserve"
	defaultServiceScopes = "listing.read"
	mdPrincipalIDKey     = "x-principal-id"
	mdPrincipalTypeKey   = "x-principal-type"
	mdPrincipalScopesKey = "x-principal-scopes"
	mdAuthorizationKey   = "authorization"
)

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

// AsService marks ctx so the client interceptor presents team-order's own service
// principal with only the stock scope (listing.write) instead of the end user's
// forwarded principal. Other incoming metadata (x-request-id, trace headers) is
// still forwarded.
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

func forwardMetadataInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		var outMD metadata.MD
		if inMD, ok := metadata.FromIncomingContext(ctx); ok {
			outMD = inMD.Copy()
		} else {
			outMD = metadata.MD{}
		}

		scope := serviceScope(ctx)
		if scope == "" {
			scope = methodScopes[method]
		}
		if scope != "" {
			// Marked RPC: never forward the buyer. Replace the principal wholesale.
			outMD.Delete(mdAuthorizationKey)
			outMD.Set(mdPrincipalIDKey, servicePrincipalID)
			outMD.Set(mdPrincipalTypeKey, servicePrincipalType)
			outMD.Set(mdPrincipalScopesKey, scope)
		} else if len(outMD.Get(mdPrincipalIDKey)) == 0 {
			// Unmarked background call with no caller: read-only service identity.
			outMD.Set(mdPrincipalIDKey, servicePrincipalID)
			outMD.Set(mdPrincipalTypeKey, servicePrincipalType)
			outMD.Set(mdPrincipalScopesKey, defaultServiceScopes)
		}
		// Otherwise: forward exactly the caller's principal, scopes untouched
		// (never widen a buyer's scopes).

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
	c.Listing = NewServiceStockClient(listingv1.NewListingServiceClient(domainConn))

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
