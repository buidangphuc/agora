package engagement

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/buidangphuc/team-engagement/internal/store"
)

var (
	// ErrInvalidRating is returned for invalid rating.
	ErrInvalidRating = errors.New("rating must be between 1 and 5")
	// ErrEmptyListing is returned for empty listing.
	ErrEmptyListing = errors.New("listing_id is required")
	// ErrEmptyReviewID is returned for empty review id.
	ErrEmptyReviewID = errors.New("review_id is required")
	// ErrEmptySeller is returned for empty seller.
	ErrEmptySeller = errors.New("seller_id is required")
)

// OrderVerifier consults team-order to confirm a buyer completed an order for a
// listing. Implemented by internal/upstream.OrderClient; a nil verifier disables
// verification (verified_purchase stays false), so the service degrades cleanly
// when UPSTREAM_ORDER_ADDR is unset.
type OrderVerifier interface {
	VerifyPurchase(ctx context.Context, buyerID, listingID, orderID string) (verified bool, sellerID string, err error)
}

// ReviewService is the review service type.
type ReviewService struct {
	repo     store.ReviewRepository
	verifier OrderVerifier
	logger   *slog.Logger
}

// NewReviewService returns a new review service.
func NewReviewService(repo store.ReviewRepository, verifier OrderVerifier, logger *slog.Logger) *ReviewService {
	if logger == nil {
		logger = slog.Default()
	}
	return &ReviewService{repo: repo, verifier: verifier, logger: logger}
}

// CreateReview implements create review for ReviewService.
func (s *ReviewService) CreateReview(
	ctx context.Context,
	listingID, userID, userName, orderID string,
	rating int32,
	comment string,
	mediaURLs []string,
) (store.Review, error) {
	if listingID == "" {
		return store.Review{}, ErrEmptyListing
	}
	if rating < 1 || rating > 5 {
		return store.Review{}, ErrInvalidRating
	}

	review := store.Review{
		ListingID: listingID,
		UserID:    userID,
		UserName:  userName,
		OrderID:   orderID,
		Rating:    rating,
		Comment:   comment,
		MediaURLs: mediaURLs,
	}

	// verified_purchase + seller_id are computed via team-order (gRPC, no DB join).
	// Any upstream failure degrades to an unverified review rather than blocking.
	if s.verifier != nil && orderID != "" {
		verified, sellerID, err := s.verifier.VerifyPurchase(ctx, userID, listingID, orderID)
		if err != nil {
			s.logger.WarnContext(ctx, "verify purchase failed; recording unverified review",
				"order_id", orderID, "err", err)
		} else {
			review.VerifiedPurchase = verified
			review.SellerID = sellerID
		}
	}

	saved, err := s.repo.CreateReview(ctx, review)
	if err != nil {
		return store.Review{}, fmt.Errorf("create review: %w", err)
	}
	return saved, nil
}

const (
	// DefaultReviewPageSize applies when the caller sends no page size.
	DefaultReviewPageSize = 20
	// MaxReviewPageSize is the largest page a caller can get; bigger requests are clamped.
	MaxReviewPageSize = 100
)

// ListReviews returns one page of a listing's reviews, newest first (id as the
// tiebreak), plus the total for the rating filter. offset < 0 is treated as 0;
// limit <= 0 uses the default and limit above the maximum is clamped.
func (s *ReviewService) ListReviews(
	ctx context.Context,
	listingID string,
	ratingFilter int32,
	offset, limit int,
) ([]store.Review, int64, error) {
	if listingID == "" {
		return nil, 0, ErrEmptyListing
	}
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 {
		limit = DefaultReviewPageSize
	}
	if limit > MaxReviewPageSize {
		limit = MaxReviewPageSize
	}
	return s.repo.ListReviews(ctx, listingID, ratingFilter, offset, limit)
}

// GetRatingSummary implements get rating summary for ReviewService.
func (s *ReviewService) GetRatingSummary(ctx context.Context, listingID string) (store.RatingSummary, error) {
	if listingID == "" {
		return store.RatingSummary{}, ErrEmptyListing
	}
	return s.repo.GetRatingSummary(ctx, listingID)
}

// MarkHelpful records a per-user "helpful" vote (idempotent) and returns the
// review's current helpful count.
func (s *ReviewService) MarkHelpful(ctx context.Context, reviewID, userID string) (int64, error) {
	if reviewID == "" {
		return 0, ErrEmptyReviewID
	}
	return s.repo.MarkHelpful(ctx, reviewID, userID)
}

// GetShopRatingSummary aggregates ratings across all of a seller's listings.
func (s *ReviewService) GetShopRatingSummary(ctx context.Context, sellerID string) (store.ShopRatingSummary, error) {
	if sellerID == "" {
		return store.ShopRatingSummary{}, ErrEmptySeller
	}
	return s.repo.GetShopRatingSummary(ctx, sellerID)
}
