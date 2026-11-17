package order

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	listingv1 "github.com/buidangphuc/team-order/generated/platform/listing/v1"
	"github.com/buidangphuc/team-order/internal/store"
	"github.com/buidangphuc/team-order/internal/upstream"
)

const maxBundleQuantity = 99

var (
	ErrBundleNotFound    = errors.New("bundle not found")
	ErrBundleUnavailable = errors.New("bundle unavailable")
	ErrBundleQuantity    = errors.New("bundle quantity must be between 1 and 99")
)

type bundleCartRepo interface {
	AddBundleLines(ctx context.Context, userID string, lines []store.CartItem) ([]store.CartItem, error)
	DropBundleTag(ctx context.Context, userID, bundleID string) ([]store.CartItem, error)
}

func cartBundleRepo(r store.CartRepository) (bundleCartRepo, error) {
	br, ok := r.(bundleCartRepo)
	if !ok {
		return nil, errors.New("cart repository has no bundle support")
	}
	return br, nil
}

func loadBundle(ctx context.Context, domain upstream.DomainClient, id string) (*listingv1.Bundle, error) {
	bc, ok := domain.(upstream.BundleClient)
	if !ok {
		return nil, errors.New("domain client has no bundle read")
	}
	resp, err := bc.GetBundle(ctx, &listingv1.GetBundleRequest{Id: id})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, ErrBundleNotFound
		}
		return nil, fmt.Errorf("get bundle from domain: %w", err)
	}
	if resp.GetBundle() == nil {
		return nil, ErrBundleNotFound
	}
	return resp.GetBundle(), nil
}

func fetchBundleListings(ctx context.Context, domain upstream.DomainClient, ids []string) ([]*listingv1.Listing, error) {
	out := make([]*listingv1.Listing, len(ids))
	errs := make([]error, len(ids))
	var wg sync.WaitGroup
	for i, id := range ids {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := domain.GetListing(ctx, &listingv1.GetListingRequest{Id: id})
			if err != nil {
				if status.Code(err) == codes.NotFound {
					errs[i] = fmt.Errorf("%w: listing missing", ErrBundleUnavailable)
				} else {
					errs[i] = fmt.Errorf("get listing %q from domain: %w", id, err)
				}
				return
			}
			if resp.GetListing() == nil {
				errs[i] = fmt.Errorf("%w: listing missing", ErrBundleUnavailable)
				return
			}
			out[i] = resp.GetListing()
		}()
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

// validateBundle checks the bundle against its current listings and returns the sum of their prices.
func validateBundle(b *listingv1.Bundle, listings []*listingv1.Listing, qty int32, checkStock bool) (int64, error) {
	if len(listings) == 0 {
		return 0, fmt.Errorf("%w: no listings", ErrBundleUnavailable)
	}
	var sum int64
	for _, l := range listings {
		switch {
		case l.GetStatus() != listingv1.ListingStatus_LISTING_STATUS_PUBLISHED:
			return 0, fmt.Errorf("%w: listing not published", ErrBundleUnavailable)
		case l.GetSellerId() != b.GetSellerId():
			return 0, fmt.Errorf("%w: listing of another seller", ErrBundleUnavailable)
		case len(l.GetVariants()) > 0:
			return 0, fmt.Errorf("%w: listing has variants", ErrBundleUnavailable)
		case checkStock && l.GetStock() < qty:
			return 0, fmt.Errorf("%w: insufficient stock", ErrBundleUnavailable)
		}
		sum += l.GetPrice()
	}
	if b.GetBundlePrice() <= 0 || b.GetBundlePrice() >= sum {
		return 0, fmt.Errorf("%w: bundle price not below the listing total", ErrBundleUnavailable)
	}
	return sum, nil
}

// allocateBundleDiscount splits (sum-bundlePrice)*qty across the listings pro rata to price,
// flooring every share and giving the remainder to the last one.
func allocateBundleDiscount(prices []int64, sum, bundlePrice int64, qty int32) []int64 {
	total := (sum - bundlePrice) * int64(qty)
	out := make([]int64, len(prices))
	var given int64
	for i, p := range prices {
		if i == len(prices)-1 {
			out[i] = total - given
			break
		}
		out[i] = total * p / sum
		given += out[i]
	}
	return out
}

func (s *CartService) AddBundleToCart(ctx context.Context, userID, bundleID string, quantity int32) ([]store.CartItem, int64, error) {
	if quantity < 1 || quantity > maxBundleQuantity {
		return nil, 0, ErrBundleQuantity
	}
	repo, err := cartBundleRepo(s.repo)
	if err != nil {
		return nil, 0, err
	}
	bundle, err := loadBundle(ctx, s.domainClient, bundleID)
	if err != nil {
		return nil, 0, err
	}
	existing, err := s.repo.GetCart(ctx, userID)
	if err != nil {
		return nil, 0, err
	}
	var already int32
	for _, it := range existing {
		if it.BundleID == bundleID && it.ListingID == bundle.GetListingIds()[0] {
			already = it.Quantity
		}
	}
	total := already + quantity

	listings, err := fetchBundleListings(ctx, s.domainClient, bundle.GetListingIds())
	if err != nil {
		return nil, 0, err
	}
	sum, err := validateBundle(bundle, listings, total, true)
	if err != nil {
		return nil, 0, err
	}
	prices := make([]int64, len(listings))
	for i, l := range listings {
		prices[i] = l.GetPrice()
	}
	discounts := allocateBundleDiscount(prices, sum, bundle.GetBundlePrice(), total)

	lines := make([]store.CartItem, len(listings))
	for i, l := range listings {
		imageURL := ""
		if len(l.GetImageKeys()) > 0 {
			imageURL = l.GetImageKeys()[0]
		}
		lines[i] = store.CartItem{
			UserID:         userID,
			ListingID:      l.GetId(),
			Quantity:       quantity,
			UnitPrice:      l.GetPrice(),
			Title:          l.GetTitle(),
			ImageURL:       imageURL,
			SellerID:       l.GetSellerId(),
			BundleID:       bundleID,
			BundleDiscount: discounts[i],
		}
	}
	if _, err := repo.AddBundleLines(ctx, userID, lines); err != nil {
		return nil, 0, err
	}
	return s.GetCart(ctx, userID)
}

// bundleOfLine returns the bundle tag of the user's line, empty for a plain or unknown line.
func (s *CartService) bundleOfLine(ctx context.Context, userID, itemID string) (string, error) {
	items, err := s.repo.GetCart(ctx, userID)
	if err != nil {
		return "", err
	}
	for _, it := range items {
		if it.ID == itemID {
			return it.BundleID, nil
		}
	}
	return "", nil
}

func (s *CartService) dropBundleTag(ctx context.Context, userID, bundleID string) error {
	if bundleID == "" {
		return nil
	}
	repo, err := cartBundleRepo(s.repo)
	if err != nil {
		return err
	}
	_, err = repo.DropBundleTag(ctx, userID, bundleID)
	return err
}

// untagInvalidBundles drops the tag of every bundle among items that no longer holds and reports
// whether any was dropped. A bundle holds when all its cart lines are selected with one common
// quantity, its listings match the lines and prices and discounts equal the current allocation.
func (s *OrderService) untagInvalidBundles(ctx context.Context, buyerID string, items []store.CartItem) (bool, error) {
	var ids []string
	seen := map[string]bool{}
	for _, it := range items {
		if it.BundleID != "" && !seen[it.BundleID] {
			seen[it.BundleID] = true
			ids = append(ids, it.BundleID)
		}
	}
	if len(ids) == 0 {
		return false, nil
	}
	repo, err := cartBundleRepo(s.cartRepo)
	if err != nil {
		return false, err
	}
	cart, err := s.cartRepo.GetCart(ctx, buyerID)
	if err != nil {
		return false, fmt.Errorf("get cart: %w", err)
	}
	selected := map[string]bool{}
	for _, it := range items {
		selected[it.ID] = true
	}
	dropped := false
	for _, id := range ids {
		var lines []store.CartItem
		whole := true
		for _, it := range cart {
			if it.BundleID == id {
				lines = append(lines, it)
				whole = whole && selected[it.ID]
			}
		}
		ok := whole
		if ok {
			ok, err = s.bundleStillHolds(ctx, id, lines)
			if err != nil {
				return false, err
			}
		}
		if ok {
			continue
		}
		if _, err := repo.DropBundleTag(ctx, buyerID, id); err != nil {
			return false, fmt.Errorf("drop bundle tag: %w", err)
		}
		dropped = true
	}
	return dropped, nil
}

func (s *OrderService) bundleStillHolds(ctx context.Context, bundleID string, lines []store.CartItem) (bool, error) {
	qty := lines[0].Quantity
	byListing := map[string]store.CartItem{}
	for _, l := range lines {
		if l.Quantity != qty {
			return false, nil
		}
		byListing[l.ListingID] = l
	}
	bundle, err := loadBundle(ctx, s.domainClient, bundleID)
	if err != nil {
		if errors.Is(err, ErrBundleNotFound) {
			return false, nil
		}
		return false, err
	}
	if len(bundle.GetListingIds()) != len(lines) {
		return false, nil
	}
	listings, err := fetchBundleListings(ctx, s.domainClient, bundle.GetListingIds())
	if err != nil {
		if errors.Is(err, ErrBundleUnavailable) {
			return false, nil
		}
		return false, err
	}
	sum, err := validateBundle(bundle, listings, qty, false)
	if err != nil {
		return false, nil
	}
	prices := make([]int64, len(listings))
	for i, l := range listings {
		prices[i] = l.GetPrice()
	}
	discounts := allocateBundleDiscount(prices, sum, bundle.GetBundlePrice(), qty)
	for i, l := range listings {
		line, ok := byListing[l.GetId()]
		if !ok || line.UnitPrice != l.GetPrice() || line.BundleDiscount != discounts[i] {
			return false, nil
		}
	}
	return true, nil
}
