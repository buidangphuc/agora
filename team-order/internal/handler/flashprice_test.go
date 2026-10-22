package handler_test

// ---- order-advertised-price task 9: checkout error mapping ----
//
// Interface fixed by these tests (package service): ErrPriceChanged,
// ErrPromotionUnavailable, OrderFlashClient and WithOrderFlashSaleClient
// (see internal/service/flashprice_test.go). The handler maps
// ErrPriceChanged -> FailedPrecondition with the exact message "price changed"
// and ErrPromotionUnavailable -> Unavailable.

import (
	"context"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	identityv1 "github.com/buidangphuc/team-order/generated/platform/identity/v1"
	listingv1 "github.com/buidangphuc/team-order/generated/platform/listing/v1"
	orderv1 "github.com/buidangphuc/team-order/generated/platform/order/v1"
	promotionv1 "github.com/buidangphuc/team-order/generated/platform/promotion/v1"
	"github.com/buidangphuc/team-order/internal/handler"
	"github.com/buidangphuc/team-order/internal/repository"
	"github.com/buidangphuc/team-order/internal/service"
	"github.com/buidangphuc/team-order/internal/upstream/upstreamtest"
)

type pricedDomain struct {
	*upstreamtest.Domain
}

func (d *pricedDomain) GetListing(_ context.Context, req *listingv1.GetListingRequest, _ ...grpc.CallOption) (*listingv1.GetListingResponse, error) {
	return &listingv1.GetListingResponse{Listing: &listingv1.Listing{Id: req.GetId(), Price: 500000, SellerId: "s1", Stock: 10}}, nil
}

// downFlash fails every promotion call like an unreachable team-promotion.
type downFlash struct {
	promotionv1.FlashSaleServiceClient
}

func (downFlash) GetActiveFlashSale(context.Context, *promotionv1.GetActiveFlashSaleRequest, ...grpc.CallOption) (*promotionv1.GetActiveFlashSaleResponse, error) {
	return nil, status.Error(codes.Unavailable, "promotion down")
}

// flashLineHandler builds a handler over a real service whose buyer cart holds one
// line priced 350000 against a regular price of 500000.
func flashLineHandler(t *testing.T, opts ...service.OrderServiceOption) *handler.OrderHandler {
	t.Helper()
	domain := &pricedDomain{Domain: upstreamtest.NewDomain(map[string]int32{"lst_1": 10})}
	carts := repository.NewInMemoryCartRepository()
	if _, err := carts.AddItem(context.Background(), repository.CartItem{
		UserID: "buyer_1", ListingID: "lst_1", Quantity: 1, UnitPrice: 350000, SellerID: "s1", Title: "item",
	}); err != nil {
		t.Fatal(err)
	}
	svc := service.NewOrderService(repository.NewInMemoryOrderRepository(), carts, nil, nil, domain, nil, nil, opts...)
	return handler.NewOrderHandler(svc, &fakeAddrClient{addrs: []*identityv1.Address{addrA()}}, nil)
}

func TestCreateOrderPriceChangedFailedPrecondition(t *testing.T) {
	// No promotion client: the 350000 line is below the 500000 regular price, so it
	// is re-priced and refused as "price changed".
	h := flashLineHandler(t)
	_, err := h.CreateOrder(incomingPrincipalCtx("buyer_1", "buyer"), &orderv1.CreateOrderRequest{})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("code = %v (%v), want FailedPrecondition", status.Code(err), err)
	}
	if got := status.Convert(err).Message(); got != "price changed" {
		t.Errorf("message = %q, want exactly %q", got, "price changed")
	}
}

func TestCreateOrderPromotionUnavailableUnavailable(t *testing.T) {
	h := flashLineHandler(t, service.WithOrderFlashSaleClient(downFlash{}))
	_, err := h.CreateOrder(incomingPrincipalCtx("buyer_1", "buyer"), &orderv1.CreateOrderRequest{})
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("code = %v (%v), want Unavailable", status.Code(err), err)
	}
}
