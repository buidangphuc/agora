package upstream

import (
	"context"
	"fmt"

	"google.golang.org/grpc"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	sdkupstream "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/upstream"

	"github.com/buidangphuc/team-order/internal/config"

	identityv1 "github.com/buidangphuc/team-order/generated/platform/identity/v1"
	listingv1 "github.com/buidangphuc/team-order/generated/platform/listing/v1"
	promotionv1 "github.com/buidangphuc/team-order/generated/platform/promotion/v1"
)

const (
	servicePrincipalID = "service-team-order"
	stockServiceScope  = "listing.write"
	promotionScope     = "promotion.reserve"
	listingReadScope   = "listing.read"
)

type Clients struct {
	conns   []*grpc.ClientConn
	Listing DomainClient
	Address identityv1.AddressServiceClient
	Voucher promotionv1.VoucherServiceClient
	Flash   promotionv1.FlashSaleServiceClient
}

func DialAll(s config.Settings) (*Clients, error) {
	c := &Clients{}
	dial := func(name, addr string) (*grpc.ClientConn, error) {
		conn, err := sdkupstream.NewClient(addr)
		if err != nil {
			c.Close()
			return nil, fmt.Errorf("dial %s %s: %w", name, addr, err)
		}
		c.conns = append(c.conns, conn)
		return conn, nil
	}
	domainConn, err := dial("domain", s.Upstream.DomainAddr)
	if err != nil {
		return nil, err
	}
	identityConn, err := dial("identity", s.Upstream.IdentityAddr)
	if err != nil {
		return nil, err
	}
	promotionConn, err := dial("promotion", s.Upstream.PromotionAddr)
	if err != nil {
		return nil, err
	}
	c.Listing = NewServiceStockClient(listingv1.NewListingServiceClient(domainConn))
	c.Address = identityv1.NewAddressServiceClient(identityConn)
	c.Voucher = NewServicePromotionClient(promotionv1.NewVoucherServiceClient(promotionConn))
	c.Flash = NewServiceFlashClient(promotionv1.NewFlashSaleServiceClient(promotionConn))
	return c, nil
}

func (c *Clients) Close() {
	for _, conn := range c.conns {
		_ = conn.Close()
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
	return c.DomainClient.ReserveStock(sdkupstream.WithServicePrincipal(ctx, servicePrincipalID, stockServiceScope), req, opts...)
}

func (c serviceStockClient) ReleaseStock(ctx context.Context, req *listingv1.ReleaseStockRequest, opts ...grpc.CallOption) (*listingv1.ReleaseStockResponse, error) {
	return c.DomainClient.ReleaseStock(sdkupstream.WithServicePrincipal(ctx, servicePrincipalID, stockServiceScope), req, opts...)
}

func (c serviceStockClient) CommitReservation(ctx context.Context, req *listingv1.CommitReservationRequest, opts ...grpc.CallOption) (*listingv1.CommitReservationResponse, error) {
	return c.DomainClient.CommitReservation(sdkupstream.WithServicePrincipal(ctx, servicePrincipalID, stockServiceScope), req, opts...)
}

func (c serviceStockClient) GetListing(ctx context.Context, req *listingv1.GetListingRequest, opts ...grpc.CallOption) (*listingv1.GetListingResponse, error) {
	if p, ok := auth.FromContext(ctx); !ok || p.Anonymous() {
		ctx = sdkupstream.WithServicePrincipal(ctx, servicePrincipalID, listingReadScope)
	}
	return c.DomainClient.GetListing(ctx, req, opts...)
}
