package rpc_test

import (
	"context"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	commonv1 "github.com/buidangphuc/team-order/generated/platform/common/v1"
	listingv1 "github.com/buidangphuc/team-order/generated/platform/listing/v1"
	orderv1 "github.com/buidangphuc/team-order/generated/platform/order/v1"
	"github.com/buidangphuc/team-order/internal/rpc"
	"github.com/buidangphuc/team-order/internal/store"
	"github.com/buidangphuc/team-order/internal/upstream/upstreamtest"
)

// Interface fixed by these tests: CartHandler.AddBundleToCart(ctx, *orderv1.AddBundleToCartRequest)
// (*orderv1.AddBundleToCartResponse, error); upstream.DomainClient gains GetBundle; the wire
// cart carries bundle_id / bundle_discount per item and bundle_discount_total, subtotal net.

type bundleDomain struct {
	*upstreamtest.Domain
	listings map[string]*listingv1.Listing
	bundles  map[string]*listingv1.Bundle
}

func (d *bundleDomain) GetListing(_ context.Context, req *listingv1.GetListingRequest, _ ...grpc.CallOption) (*listingv1.GetListingResponse, error) {
	l, ok := d.listings[req.GetId()]
	if !ok {
		return nil, status.Error(codes.NotFound, "listing not found")
	}
	cp := proto.Clone(l).(*listingv1.Listing)
	return &listingv1.GetListingResponse{Listing: cp}, nil
}

func (d *bundleDomain) GetBundle(_ context.Context, req *listingv1.GetBundleRequest, _ ...grpc.CallOption) (*listingv1.GetBundleResponse, error) {
	b, ok := d.bundles[req.GetId()]
	if !ok {
		return nil, status.Error(codes.NotFound, "bundle not found")
	}
	cp := proto.Clone(b).(*listingv1.Bundle)
	return &listingv1.GetBundleResponse{Bundle: cp}, nil
}

func bundleListing(id string, price int64) *listingv1.Listing {
	return &listingv1.Listing{Id: id, Title: id, Price: price, SellerId: "s1", Stock: 20, Status: listingv1.ListingStatus_LISTING_STATUS_PUBLISHED}
}

type bundleRPCRig struct {
	h      *rpc.CartHandler
	carts  *store.PostgresCartRepository
	domain *bundleDomain
}

func newBundleRPCRig(t *testing.T) *bundleRPCRig {
	t.Helper()
	pg := newPGOutbox(t.Context(), t)
	r := &bundleRPCRig{carts: store.NewPostgresCartRepository(pg.pool)}
	r.domain = &bundleDomain{
		Domain: upstreamtest.NewDomain(map[string]int32{"lst_a": 20, "lst_b": 20}),
		listings: map[string]*listingv1.Listing{
			"lst_a": bundleListing("lst_a", 100000),
			"lst_b": bundleListing("lst_b", 50000),
		},
		bundles: map[string]*listingv1.Bundle{
			"bnd_ab": {Id: "bnd_ab", SellerId: "s1", ListingIds: []string{"lst_a", "lst_b"}, BundlePrice: 120000},
		},
	}
	r.h = rpc.NewCartHandler(newCart(t, r.carts, pg.orders(), r.domain, discardLogger()), discardLogger())
	return r
}

func wireItem(c *orderv1.Cart, listing string) *orderv1.CartItem {
	for _, it := range c.GetItems() {
		if it.GetListingId() == listing {
			return it
		}
	}
	return nil
}

// Scenario: Adding a bundle creates one tagged line per listing
// Scenario: The discount is allocated exactly with the remainder on the last line
func TestAddBundleToCartWireCart(t *testing.T) {
	r := newBundleRPCRig(t)
	resp, err := r.h.AddBundleToCart(incomingPrincipalCtx("buyer_1", "buyer"), &orderv1.AddBundleToCartRequest{BundleId: "bnd_ab", Quantity: 1})
	if err != nil {
		t.Fatal(err)
	}
	c := resp.GetCart()
	a, b := wireItem(c, "lst_a"), wireItem(c, "lst_b")
	if a == nil || b == nil || a.GetBundleId() != "bnd_ab" || b.GetBundleId() != "bnd_ab" {
		t.Fatalf("items not tagged: %+v", c.GetItems())
	}
	if a.GetBundleDiscount() != 20000 || b.GetBundleDiscount() != 10000 {
		t.Errorf("discounts = %d/%d, want 20000/10000", a.GetBundleDiscount(), b.GetBundleDiscount())
	}
	if c.GetBundleDiscountTotal() != 30000 || c.GetSubtotal() != 120000 {
		t.Errorf("discount total/subtotal = %d/%d, want 30000/120000", c.GetBundleDiscountTotal(), c.GetSubtotal())
	}
	got, err := r.h.GetCart(incomingPrincipalCtx("buyer_1", "buyer"), &orderv1.GetCartRequest{})
	if err != nil || got.GetCart().GetBundleDiscountTotal() != 30000 || got.GetCart().GetSubtotal() != 120000 {
		t.Fatalf("GetCart = %+v %v", got.GetCart(), err)
	}
}

// Scenario: A quantity out of range is rejected
func TestAddBundleToCartQuantityOutOfRangeInvalidArgument(t *testing.T) {
	r := newBundleRPCRig(t)
	ctx := incomingPrincipalCtx("buyer_1", "buyer")
	for _, q := range []int32{0, 100} {
		_, err := r.h.AddBundleToCart(ctx, &orderv1.AddBundleToCartRequest{BundleId: "bnd_ab", Quantity: q})
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("quantity %d: code = %v, want InvalidArgument", q, status.Code(err))
		}
	}
	if items, _ := r.carts.GetCart(context.Background(), "buyer_1"); len(items) != 0 {
		t.Fatalf("cart changed: %+v", items)
	}
}

// Scenario: A quantity out of range is rejected
func TestAddBundleToCartEmptyBundleIDInvalidArgument(t *testing.T) {
	r := newBundleRPCRig(t)
	_, err := r.h.AddBundleToCart(incomingPrincipalCtx("buyer_1", "buyer"), &orderv1.AddBundleToCartRequest{Quantity: 1})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("code = %v, want InvalidArgument", status.Code(err))
	}
}

// Scenario: An unknown bundle is rejected
func TestAddBundleToCartUnknownBundleNotFound(t *testing.T) {
	r := newBundleRPCRig(t)
	_, err := r.h.AddBundleToCart(incomingPrincipalCtx("buyer_1", "buyer"), &orderv1.AddBundleToCartRequest{BundleId: "bnd_nope", Quantity: 1})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("code = %v, want NotFound", status.Code(err))
	}
}

// Scenario: A bundle with an unavailable listing is refused
// Scenario: A bundle with a variant listing is refused
// Scenario: A bundle priced at or above the sum of its listings is refused
func TestAddBundleToCartRefusalsAreFailedPrecondition(t *testing.T) {
	cases := map[string]func(d *bundleDomain){
		"unpublished": func(d *bundleDomain) {
			d.listings["lst_b"].Status = listingv1.ListingStatus_LISTING_STATUS_DRAFT
		},
		"out of stock": func(d *bundleDomain) { d.listings["lst_b"].Stock = 0 },
		"other owner":  func(d *bundleDomain) { d.listings["lst_b"].SellerId = "s2" },
		"variant": func(d *bundleDomain) {
			d.listings["lst_b"].Variants = []*listingv1.Variant{{Id: "v1", Name: "Red", Price: 50000, Stock: 3}}
		},
		"price equal to sum": func(d *bundleDomain) { d.bundles["bnd_ab"].BundlePrice = 150000 },
		"price above sum":    func(d *bundleDomain) { d.bundles["bnd_ab"].BundlePrice = 160000 },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			r := newBundleRPCRig(t)
			mutate(r.domain)
			_, err := r.h.AddBundleToCart(incomingPrincipalCtx("buyer_1", "buyer"), &orderv1.AddBundleToCartRequest{BundleId: "bnd_ab", Quantity: 1})
			if status.Code(err) != codes.FailedPrecondition {
				t.Fatalf("code = %v (%v), want FailedPrecondition", status.Code(err), err)
			}
			if items, _ := r.carts.GetCart(context.Background(), "buyer_1"); len(items) != 0 {
				t.Fatalf("cart changed: %+v", items)
			}
		})
	}
}

// Scenario: An unauthenticated caller is rejected
func TestAddBundleToCartAnonymousUnauthenticated(t *testing.T) {
	r := newBundleRPCRig(t)
	anon := withPrincipal(context.Background(), &commonv1.Principal{Id: "anonymous", Type: commonv1.PrincipalType_PRINCIPAL_TYPE_ANONYMOUS})
	for name, ctx := range map[string]context.Context{"anonymous": anon, "no principal": context.Background()} {
		_, err := r.h.AddBundleToCart(ctx, &orderv1.AddBundleToCartRequest{BundleId: "bnd_ab", Quantity: 1})
		if status.Code(err) != codes.Unauthenticated {
			t.Errorf("%s: code = %v, want Unauthenticated", name, status.Code(err))
		}
	}
}

// Scenario: The cart of another buyer is never touched
func TestAddBundleToCartOtherBuyerCartUntouched(t *testing.T) {
	r := newBundleRPCRig(t)
	if _, err := r.h.AddBundleToCart(incomingPrincipalCtx("buyer_A", "buyer"), &orderv1.AddBundleToCartRequest{BundleId: "bnd_ab", Quantity: 1}); err != nil {
		t.Fatal(err)
	}
	got, err := r.h.GetCart(incomingPrincipalCtx("buyer_B", "buyer"), &orderv1.GetCartRequest{})
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range got.GetCart().GetItems() {
		if it.GetBundleId() != "" {
			t.Fatalf("buyer B sees a bundle line: %+v", it)
		}
	}
}

// Scenario: Changing one line quantity drops the tag from all lines
// Scenario: Removing one line drops the tag from the others
func TestCartBundleTagDropOverWire(t *testing.T) {
	r := newBundleRPCRig(t)
	ctx := incomingPrincipalCtx("buyer_1", "buyer")
	resp, err := r.h.AddBundleToCart(ctx, &orderv1.AddBundleToCartRequest{BundleId: "bnd_ab", Quantity: 1})
	if err != nil {
		t.Fatal(err)
	}
	line := wireItem(resp.GetCart(), "lst_a")
	up, err := r.h.UpdateCartItem(ctx, &orderv1.UpdateCartItemRequest{ItemId: line.GetId(), Quantity: 2})
	if err != nil {
		t.Fatal(err)
	}
	if up.GetCart().GetBundleDiscountTotal() != 0 || up.GetCart().GetSubtotal() != 2*100000+50000 {
		t.Errorf("after update: discount total %d subtotal %d", up.GetCart().GetBundleDiscountTotal(), up.GetCart().GetSubtotal())
	}
	for _, it := range up.GetCart().GetItems() {
		if it.GetBundleId() != "" || it.GetBundleDiscount() != 0 {
			t.Errorf("still tagged: %+v", it)
		}
	}

	r2 := newBundleRPCRig(t)
	resp, err = r2.h.AddBundleToCart(ctx, &orderv1.AddBundleToCartRequest{BundleId: "bnd_ab", Quantity: 1})
	if err != nil {
		t.Fatal(err)
	}
	rm, err := r2.h.RemoveFromCart(ctx, &orderv1.RemoveFromCartRequest{ItemId: wireItem(resp.GetCart(), "lst_a").GetId()})
	if err != nil {
		t.Fatal(err)
	}
	if len(rm.GetCart().GetItems()) != 1 || rm.GetCart().GetBundleDiscountTotal() != 0 || rm.GetCart().GetItems()[0].GetBundleId() != "" {
		t.Fatalf("after remove: %+v", rm.GetCart())
	}
}

// Scenario: Another buyer's bundle line cannot be changed
func TestCartBundleLineOfAnotherBuyerNotFound(t *testing.T) {
	r := newBundleRPCRig(t)
	resp, err := r.h.AddBundleToCart(incomingPrincipalCtx("buyer_A", "buyer"), &orderv1.AddBundleToCartRequest{BundleId: "bnd_ab", Quantity: 1})
	if err != nil {
		t.Fatal(err)
	}
	id := wireItem(resp.GetCart(), "lst_a").GetId()
	b := incomingPrincipalCtx("buyer_B", "buyer")
	if _, err := r.h.UpdateCartItem(b, &orderv1.UpdateCartItemRequest{ItemId: id, Quantity: 5}); status.Code(err) != codes.NotFound {
		t.Errorf("update: code = %v, want NotFound", status.Code(err))
	}
	if _, err := r.h.RemoveFromCart(b, &orderv1.RemoveFromCartRequest{ItemId: id}); status.Code(err) != codes.NotFound {
		t.Errorf("remove: code = %v, want NotFound", status.Code(err))
	}
	got, _ := r.h.GetCart(incomingPrincipalCtx("buyer_A", "buyer"), &orderv1.GetCartRequest{})
	if got.GetCart().GetBundleDiscountTotal() != 30000 {
		t.Fatalf("buyer A bundle changed: %+v", got.GetCart())
	}
}
