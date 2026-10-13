// Package order holds team-order's business logic: carts, checkout and the
// purchase saga, order status changes, cancellation, returns and shipments. It
// returns plain Go errors; internal/rpc maps them to gRPC codes.
package order

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	listingv1 "github.com/buidangphuc/team-order/generated/platform/listing/v1"
	"github.com/buidangphuc/team-order/internal/store"
	"github.com/buidangphuc/team-order/internal/upstream"
)

// ErrReorderNotOwner is returned when a caller tries to reorder an order that
// belongs to another user. Reorder is owner-scoped: only the buyer who placed
// the order may re-add its items to their cart.
var ErrReorderNotOwner = errors.New("cannot reorder another user's order")

// CartService holds the cart use cases: read, edit and re-order.
type CartService struct {
	repo         store.CartRepository
	orderRepo    store.OrderRepository
	domainClient upstream.DomainClient
	logger       *slog.Logger
}

// NewCartService builds the cart service over its repositories and the listing client.
func NewCartService(repo store.CartRepository, orderRepo store.OrderRepository, domainClient upstream.DomainClient, logger *slog.Logger) *CartService {
	if logger == nil {
		logger = slog.Default()
	}
	return &CartService{
		repo:         repo,
		orderRepo:    orderRepo,
		domainClient: domainClient,
		logger:       logger,
	}
}

// GetCart returns the user's cart items and their subtotal.
func (s *CartService) GetCart(ctx context.Context, userID string) ([]store.CartItem, int64, error) {
	items, err := s.repo.GetCart(ctx, userID)
	if err != nil {
		return nil, 0, err
	}
	var subtotal int64
	for _, it := range items {
		subtotal += it.UnitPrice * int64(it.Quantity)
	}
	return items, subtotal, nil
}

// AddToCart adds a listing (variant) to the cart after checking it against team-domain.
func (s *CartService) AddToCart(ctx context.Context, userID, listingID, variantID string, quantity int32) ([]store.CartItem, int64, error) {
	if quantity <= 0 {
		quantity = 1
	}

	// Fetch listing info from domain service
	resp, err := s.domainClient.GetListing(ctx, &listingv1.GetListingRequest{Id: listingID})
	if err != nil {
		return nil, 0, fmt.Errorf("get listing %q from domain: %w", listingID, err)
	}
	listing := resp.GetListing()
	if listing == nil {
		return nil, 0, fmt.Errorf("listing %q not found", listingID)
	}

	unitPrice := listing.GetPrice()
	title := listing.GetTitle()
	variantName := ""
	imageURL := ""
	if len(listing.GetImageKeys()) > 0 {
		imageURL = listing.GetImageKeys()[0]
	}

	if variantID != "" {
		for _, v := range listing.GetVariants() {
			if v.GetId() == variantID {
				variantName = v.GetName()
				if v.GetPrice() > 0 {
					unitPrice = v.GetPrice()
				}
				if v.GetImageUrl() != "" {
					imageURL = v.GetImageUrl()
				}
				break
			}
		}
	}

	sellerID := listing.GetSellerId()

	item := store.CartItem{
		UserID:      userID,
		ListingID:   listingID,
		VariantID:   variantID,
		Quantity:    quantity,
		UnitPrice:   unitPrice,
		Title:       title,
		VariantName: variantName,
		ImageURL:    imageURL,
		SellerID:    sellerID,
	}

	if _, err := s.repo.AddItem(ctx, item); err != nil {
		return nil, 0, err
	}

	return s.GetCart(ctx, userID)
}

// UpdateCartItem sets the quantity of a cart item.
func (s *CartService) UpdateCartItem(ctx context.Context, userID, itemID string, quantity int32) ([]store.CartItem, int64, error) {
	if _, err := s.repo.UpdateItem(ctx, userID, itemID, quantity); err != nil {
		return nil, 0, err
	}
	return s.GetCart(ctx, userID)
}

// RemoveFromCart removes one item from the cart.
func (s *CartService) RemoveFromCart(ctx context.Context, userID, itemID string) ([]store.CartItem, int64, error) {
	if _, err := s.repo.RemoveItem(ctx, userID, itemID); err != nil {
		return nil, 0, err
	}
	return s.GetCart(ctx, userID)
}

// ClearCart empties the user's cart.
func (s *CartService) ClearCart(ctx context.Context, userID string) error {
	return s.repo.ClearCart(ctx, userID)
}

// Reorder ("Mua lại") loads a past order owned by the caller and re-adds each of
// its items to the caller's cart via the existing AddToCart path, then returns
// the updated cart. It is owner-scoped: only the buyer who placed the order may
// reorder it. Re-adding through AddToCart re-prices every line at the current
// listing price (mirrors AddToCart) rather than trusting the historical snapshot.
func (s *CartService) Reorder(ctx context.Context, userID, orderID string) ([]store.CartItem, int64, error) {
	if orderID == "" {
		// Treat an empty id as "no such order" so anonymous/empty input never panics.
		return nil, 0, store.ErrOrderNotFound
	}
	if s.orderRepo == nil {
		return nil, 0, fmt.Errorf("reorder: order repository not configured")
	}

	order, err := s.orderRepo.GetOrder(ctx, orderID)
	if err != nil {
		return nil, 0, err
	}

	// Owner-scoped: a buyer may only reorder their own past order.
	if order.BuyerID != userID {
		return nil, 0, ErrReorderNotOwner
	}

	// Re-add each item through the existing cart add path so pricing, variant
	// resolution and the upsert/merge semantics are identical to AddToCart.
	for _, it := range order.Items {
		if it.ListingID == "" {
			continue
		}
		if _, _, err := s.AddToCart(ctx, userID, it.ListingID, it.VariantID, it.Quantity); err != nil {
			return nil, 0, fmt.Errorf("reorder add item %q: %w", it.ListingID, err)
		}
	}

	return s.GetCart(ctx, userID)
}
