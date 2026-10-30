package order_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"google.golang.org/grpc"

	listingv1 "github.com/buidangphuc/team-order/generated/platform/listing/v1"
	promotionv1 "github.com/buidangphuc/team-order/generated/platform/promotion/v1"
	"github.com/buidangphuc/team-order/internal/order"
	"github.com/buidangphuc/team-order/internal/store"
	"github.com/buidangphuc/team-order/internal/store/fake"
)

// reorderFakeDomain implements upstream.DomainClient. GetListing returns a
// listing priced by listing_id so the re-add path (AddToCart) can resolve a
// current price; ReserveStock/ReleaseStock are unused by Reorder.
type reorderFakeDomain struct {
	price int64
}

func (f *reorderFakeDomain) GetListing(_ context.Context, req *listingv1.GetListingRequest, _ ...grpc.CallOption) (*listingv1.GetListingResponse, error) {
	return &listingv1.GetListingResponse{
		Listing: &listingv1.Listing{
			Id:       req.GetId(),
			Title:    "Sản phẩm " + req.GetId(),
			Price:    f.price,
			SellerId: "seller_1",
		},
	}, nil
}

func (f *reorderFakeDomain) ReserveStock(_ context.Context, _ *listingv1.ReserveStockRequest, _ ...grpc.CallOption) (*listingv1.ReserveStockResponse, error) {
	return &listingv1.ReserveStockResponse{}, nil
}

func (f *reorderFakeDomain) CommitReservation(_ context.Context, _ *listingv1.CommitReservationRequest, _ ...grpc.CallOption) (*listingv1.CommitReservationResponse, error) {
	return &listingv1.CommitReservationResponse{}, nil
}

func (f *reorderFakeDomain) ReleaseStock(_ context.Context, _ *listingv1.ReleaseStockRequest, _ ...grpc.CallOption) (*listingv1.ReleaseStockResponse, error) {
	return &listingv1.ReleaseStockResponse{}, nil
}

func newReorderCartService(t *testing.T, orders map[string]store.Order) *order.CartService {
	t.Helper()
	orderRepo := &mockOrderServiceRepo{orders: orders}
	cartRepo := fake.NewInMemoryCartRepository()
	return newCart(t, cartRepo, orderRepo, &reorderFakeDomain{price: 99000}, nil)
}

func TestCartService_Reorder_ReAddsItems(t *testing.T) {
	ctx := context.Background()
	svc := newReorderCartService(t, map[string]store.Order{
		"ord_1": {
			ID:      "ord_1",
			BuyerID: "buyer_1",
			Items: []store.OrderItem{
				{ListingID: "lst_a", VariantID: "", Quantity: 2},
				{ListingID: "lst_b", VariantID: "v1", Quantity: 1},
			},
		},
	})

	items, subtotal, err := svc.Reorder(ctx, "buyer_1", "ord_1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 cart items after reorder, got %d", len(items))
	}
	// Every re-added line is owned by the caller and re-priced at current price.
	got := map[string]int32{}
	for _, it := range items {
		if it.UserID != "buyer_1" {
			t.Errorf("cart item owned by %q, want buyer_1", it.UserID)
		}
		if it.UnitPrice != 99000 {
			t.Errorf("expected re-priced unit price 99000, got %d", it.UnitPrice)
		}
		got[it.ListingID] = it.Quantity
	}
	if got["lst_a"] != 2 || got["lst_b"] != 1 {
		t.Errorf("unexpected quantities: %+v", got)
	}
	if subtotal != 99000*3 {
		t.Errorf("expected subtotal %d, got %d", 99000*3, subtotal)
	}
}

func TestCartService_Reorder_NotYourOrder(t *testing.T) {
	ctx := context.Background()
	svc := newReorderCartService(t, map[string]store.Order{
		"ord_2": {
			ID:      "ord_2",
			BuyerID: "buyer_2",
			Items:   []store.OrderItem{{ListingID: "lst_a", Quantity: 1}},
		},
	})

	_, _, err := svc.Reorder(ctx, "buyer_1", "ord_2")
	if !errors.Is(err, order.ErrReorderNotOwner) {
		t.Fatalf("expected ErrReorderNotOwner, got %v", err)
	}

	// The other user's cart must remain untouched by the rejected reorder.
	items, _, err := svc.GetCart(ctx, "buyer_1")
	if err != nil {
		t.Fatalf("unexpected error reading cart: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected empty cart after rejected reorder, got %d items", len(items))
	}
}

func TestCartService_Reorder_MissingOrder(t *testing.T) {
	ctx := context.Background()
	svc := newReorderCartService(t, map[string]store.Order{})

	_, _, err := svc.Reorder(ctx, "buyer_1", "ord_missing")
	if !errors.Is(err, store.ErrOrderNotFound) {
		t.Fatalf("expected ErrOrderNotFound for missing order, got %v", err)
	}
}

func TestCartService_Reorder_EmptyOrderID(t *testing.T) {
	ctx := context.Background()
	svc := newReorderCartService(t, map[string]store.Order{})

	// Empty/anonymous input must be handled without panic.
	_, _, err := svc.Reorder(ctx, "buyer_1", "")
	if !errors.Is(err, store.ErrOrderNotFound) {
		t.Fatalf("expected ErrOrderNotFound for empty order id, got %v", err)
	}
}

// ---- order-advertised-price: AddToCart / Reorder at the active flash price ----
//
// Interface fixed by these tests (implementer must provide):
//   - order.WithFlashSaleClient(c order.FlashSaleClient) order.CartOption
//   - newCart(t, repo, orderRepo, domain, logger, opts ...order.CartOption)
//     (variadic; existing 4-arg calls keep compiling)
//   - order.FlashSaleClient: an interface that promotionv1.FlashSaleServiceClient
//     satisfies (it must be assignable from the fake below, which embeds the full
//     promotionv1.FlashSaleServiceClient and implements only GetActiveFlashSale).
//   - WithFlashSaleClient(nil) and a typed-nil are treated as "no client".
//   - AddToCart makes exactly one GetActiveFlashSale(listing_id) call per add.

// flashDomain prices listings from a table and supports variants.
type flashDomain struct {
	listings map[string]*listingv1.Listing
}

func (f *flashDomain) GetListing(_ context.Context, req *listingv1.GetListingRequest, _ ...grpc.CallOption) (*listingv1.GetListingResponse, error) {
	l, ok := f.listings[req.GetId()]
	if !ok {
		return nil, errors.New("not found")
	}
	return &listingv1.GetListingResponse{Listing: l}, nil
}

func (f *flashDomain) ReserveStock(context.Context, *listingv1.ReserveStockRequest, ...grpc.CallOption) (*listingv1.ReserveStockResponse, error) {
	return &listingv1.ReserveStockResponse{}, nil
}

func (f *flashDomain) CommitReservation(context.Context, *listingv1.CommitReservationRequest, ...grpc.CallOption) (*listingv1.CommitReservationResponse, error) {
	return &listingv1.CommitReservationResponse{}, nil
}

func (f *flashDomain) ReleaseStock(context.Context, *listingv1.ReleaseStockRequest, ...grpc.CallOption) (*listingv1.ReleaseStockResponse, error) {
	return &listingv1.ReleaseStockResponse{}, nil
}

type flashFake struct {
	promotionv1.FlashSaleServiceClient
	byListing map[string]*promotionv1.GetActiveFlashSaleResponse
	err       error
	calls     []string
}

func (f *flashFake) GetActiveFlashSale(_ context.Context, req *promotionv1.GetActiveFlashSaleRequest, _ ...grpc.CallOption) (*promotionv1.GetActiveFlashSaleResponse, error) {
	f.calls = append(f.calls, req.GetListingId())
	if f.err != nil {
		return nil, f.err
	}
	if r, ok := f.byListing[req.GetListingId()]; ok {
		return r, nil
	}
	return &promotionv1.GetActiveFlashSaleResponse{}, nil
}

func activeSale(variantID string, price int64) *promotionv1.GetActiveFlashSaleResponse {
	return &promotionv1.GetActiveFlashSaleResponse{
		Active:   true,
		Campaign: &promotionv1.FlashSaleCampaign{Id: "c1", VariantId: variantID, SalePrice: price, StockCap: 10},
	}
}

func newFlashCart(t testing.TB, dom *flashDomain, orders map[string]store.Order, opts ...order.CartOption) *order.CartService {
	return newCart(t, fake.NewInMemoryCartRepository(),
		&mockOrderServiceRepo{orders: orders}, dom, nil, opts...)
}

func linePrice(t *testing.T, items []store.CartItem, listing, variant string) int64 {
	t.Helper()
	for _, it := range items {
		if it.ListingID == listing && it.VariantID == variant {
			return it.UnitPrice
		}
	}
	t.Fatalf("no cart line %s/%s in %+v", listing, variant, items)
	return 0
}

func TestAddToCartFlashPriceListingWide(t *testing.T) {
	dom := &flashDomain{listings: map[string]*listingv1.Listing{"l1": {Id: "l1", Price: 500000, SellerId: "s"}}}
	fl := &flashFake{byListing: map[string]*promotionv1.GetActiveFlashSaleResponse{"l1": activeSale("", 350000)}}
	svc := newFlashCart(t, dom, nil, order.WithFlashSaleClient(fl))

	items, subtotal, err := svc.AddToCart(context.Background(), "b1", "l1", "", 2)
	if err != nil {
		t.Fatal(err)
	}
	if got := linePrice(t, items, "l1", ""); got != 350000 {
		t.Errorf("unit_price = %d, want 350000", got)
	}
	if subtotal != 700000 {
		t.Errorf("subtotal = %d, want 700000", subtotal)
	}
	if len(fl.calls) != 1 || fl.calls[0] != "l1" {
		t.Errorf("GetActiveFlashSale calls = %v, want exactly [l1]", fl.calls)
	}
}

func TestAddToCartFlashPriceVariantOnly(t *testing.T) {
	dom := &flashDomain{listings: map[string]*listingv1.Listing{"l1": {
		Id: "l1", Price: 400000, SellerId: "s",
		Variants: []*listingv1.Variant{{Id: "v1", Name: "V1", Price: 400000}, {Id: "v2", Name: "V2", Price: 400000}},
	}}}
	fl := &flashFake{byListing: map[string]*promotionv1.GetActiveFlashSaleResponse{"l1": activeSale("v1", 300000)}}
	svc := newFlashCart(t, dom, nil, order.WithFlashSaleClient(fl))

	ctx := context.Background()
	if _, _, err := svc.AddToCart(ctx, "b1", "l1", "v1", 1); err != nil {
		t.Fatal(err)
	}
	items, _, err := svc.AddToCart(ctx, "b1", "l1", "v2", 1)
	if err != nil {
		t.Fatal(err)
	}
	if got := linePrice(t, items, "l1", "v1"); got != 300000 {
		t.Errorf("v1 unit_price = %d, want 300000", got)
	}
	if got := linePrice(t, items, "l1", "v2"); got != 400000 {
		t.Errorf("v2 unit_price = %d, want 400000", got)
	}
}

func TestAddToCartFlashPriceNotBelowRegularIgnored(t *testing.T) {
	dom := &flashDomain{listings: map[string]*listingv1.Listing{
		"eq":   {Id: "eq", Price: 200000, SellerId: "s"},
		"hi":   {Id: "hi", Price: 200000, SellerId: "s"},
		"zero": {Id: "zero", Price: 200000, SellerId: "s"},
	}}
	fl := &flashFake{byListing: map[string]*promotionv1.GetActiveFlashSaleResponse{
		"eq":   activeSale("", 200000),
		"hi":   activeSale("", 250000),
		"zero": activeSale("", 0),
	}}
	svc := newFlashCart(t, dom, nil, order.WithFlashSaleClient(fl))
	ctx := context.Background()
	var items []store.CartItem
	var err error
	for _, l := range []string{"eq", "hi", "zero"} {
		if items, _, err = svc.AddToCart(ctx, "b1", l, "", 1); err != nil {
			t.Fatal(err)
		}
	}
	for _, l := range []string{"eq", "hi", "zero"} {
		if got := linePrice(t, items, l, ""); got != 200000 {
			t.Errorf("%s unit_price = %d, want regular 200000", l, got)
		}
	}
}

func TestAddToCartNoCampaignRegularPrice(t *testing.T) {
	dom := &flashDomain{listings: map[string]*listingv1.Listing{"l1": {Id: "l1", Price: 123000, SellerId: "s"}}}
	// Active=false even though a campaign body is present: must be ignored.
	fl := &flashFake{byListing: map[string]*promotionv1.GetActiveFlashSaleResponse{"l1": {
		Active: false, Campaign: &promotionv1.FlashSaleCampaign{SalePrice: 1000},
	}}}
	svc := newFlashCart(t, dom, nil, order.WithFlashSaleClient(fl))
	items, _, err := svc.AddToCart(context.Background(), "b1", "l1", "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if got := linePrice(t, items, "l1", ""); got != 123000 {
		t.Errorf("unit_price = %d, want 123000", got)
	}
}

func TestAddToCartPromotionErrorRegularPrice(t *testing.T) {
	dom := &flashDomain{listings: map[string]*listingv1.Listing{"l1": {Id: "l1", Price: 123000, SellerId: "s"}}}
	fl := &flashFake{err: errors.New("promotion down")}
	svc := newFlashCart(t, dom, nil, order.WithFlashSaleClient(fl))
	items, _, err := svc.AddToCart(context.Background(), "b1", "l1", "", 1)
	if err != nil {
		t.Fatalf("AddToCart must succeed on promotion error: %v", err)
	}
	if got := linePrice(t, items, "l1", ""); got != 123000 {
		t.Errorf("unit_price = %d, want 123000", got)
	}
}

func TestNewCartServiceRefusesNilFlashClient(t *testing.T) {
	dom := &flashDomain{listings: map[string]*listingv1.Listing{"l1": {Id: "l1", Price: 123000, SellerId: "s"}}}
	var typedNil *flashFake
	for name, opts := range map[string][]order.CartOption{
		"no option": nil,
		"nil":       {order.WithFlashSaleClient(nil)},
		"typed-nil": {order.WithFlashSaleClient(typedNil)},
	} {
		t.Run(name, func(t *testing.T) {
			svc, err := order.NewCartService(fake.NewInMemoryCartRepository(), &mockOrderServiceRepo{}, dom, nil, opts...)
			if err == nil || svc != nil {
				t.Fatalf("Advertised price: A cart service without a promotion client is refused: svc=%v err=%v", svc, err)
			}
			if !strings.Contains(strings.ToLower(err.Error()), "flash") {
				t.Errorf("Advertised price: A cart service without a promotion client is refused: error must name the flash client, got %q", err)
			}
		})
	}
}

func TestReorderUsesFlashPrice(t *testing.T) {
	dom := &flashDomain{listings: map[string]*listingv1.Listing{"l1": {Id: "l1", Price: 500000, SellerId: "s"}}}
	fl := &flashFake{byListing: map[string]*promotionv1.GetActiveFlashSaleResponse{"l1": activeSale("", 350000)}}
	svc := newFlashCart(t, dom, map[string]store.Order{
		"o1": {ID: "o1", BuyerID: "b1", Items: []store.OrderItem{{ListingID: "l1", Quantity: 1, UnitPrice: 500000}}},
	}, order.WithFlashSaleClient(fl))

	items, subtotal, err := svc.Reorder(context.Background(), "b1", "o1")
	if err != nil {
		t.Fatal(err)
	}
	if got := linePrice(t, items, "l1", ""); got != 350000 {
		t.Errorf("unit_price = %d, want 350000", got)
	}
	if subtotal != 350000 {
		t.Errorf("subtotal = %d, want 350000", subtotal)
	}
}
