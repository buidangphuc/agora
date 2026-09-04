package service_test

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc"

	listingv1 "github.com/buidangphuc/team-order/generated/platform/listing/v1"
	"github.com/buidangphuc/team-order/internal/repository"
	"github.com/buidangphuc/team-order/internal/service"
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

func (f *reorderFakeDomain) ReleaseStock(_ context.Context, _ *listingv1.ReleaseStockRequest, _ ...grpc.CallOption) (*listingv1.ReleaseStockResponse, error) {
	return &listingv1.ReleaseStockResponse{}, nil
}

func newReorderCartService(t *testing.T, orders map[string]repository.Order) *service.CartService {
	t.Helper()
	orderRepo := &mockOrderServiceRepo{orders: orders}
	cartRepo := repository.NewInMemoryCartRepository()
	return service.NewCartService(cartRepo, orderRepo, &reorderFakeDomain{price: 99000}, nil)
}

func TestCartService_Reorder_ReAddsItems(t *testing.T) {
	ctx := context.Background()
	svc := newReorderCartService(t, map[string]repository.Order{
		"ord_1": {
			ID:      "ord_1",
			BuyerID: "buyer_1",
			Items: []repository.OrderItem{
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
	svc := newReorderCartService(t, map[string]repository.Order{
		"ord_2": {
			ID:      "ord_2",
			BuyerID: "buyer_2",
			Items:   []repository.OrderItem{{ListingID: "lst_a", Quantity: 1}},
		},
	})

	_, _, err := svc.Reorder(ctx, "buyer_1", "ord_2")
	if !errors.Is(err, service.ErrReorderNotOwner) {
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
	svc := newReorderCartService(t, map[string]repository.Order{})

	_, _, err := svc.Reorder(ctx, "buyer_1", "ord_missing")
	if !errors.Is(err, repository.ErrOrderNotFound) {
		t.Fatalf("expected ErrOrderNotFound for missing order, got %v", err)
	}
}

func TestCartService_Reorder_EmptyOrderID(t *testing.T) {
	ctx := context.Background()
	svc := newReorderCartService(t, map[string]repository.Order{})

	// Empty/anonymous input must be handled without panic.
	_, _, err := svc.Reorder(ctx, "buyer_1", "")
	if !errors.Is(err, repository.ErrOrderNotFound) {
		t.Fatalf("expected ErrOrderNotFound for empty order id, got %v", err)
	}
}
