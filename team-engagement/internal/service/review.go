package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/buidangphuc/team-engagement/internal/repository"
)

var (
	ErrInvalidRating = errors.New("rating must be between 1 and 5")
	ErrEmptyListing  = errors.New("listing_id is required")
	ErrEmptyReviewID = errors.New("review_id is required")
	ErrEmptySeller   = errors.New("seller_id is required")
)

// OrderVerifier consults team-order to confirm a buyer completed an order for a
// listing. Implemented by internal/upstream.OrderClient; a nil verifier disables
// verification (verified_purchase stays false), so the service degrades cleanly
// when UPSTREAM_ORDER_ADDR is unset.
type OrderVerifier interface {
	VerifyPurchase(ctx context.Context, buyerID, listingID, orderID string) (verified bool, sellerID string, err error)
}

type ReviewService struct {
	repo     repository.ReviewRepository
	verifier OrderVerifier
	logger   *slog.Logger
}

func NewReviewService(repo repository.ReviewRepository, verifier OrderVerifier, logger *slog.Logger) *ReviewService {
	if logger == nil {
		logger = slog.Default()
	}
	return &ReviewService{repo: repo, verifier: verifier, logger: logger}
}

func (s *ReviewService) CreateReview(
	ctx context.Context,
	listingID, userID, userName, orderID string,
	rating int32,
	comment string,
	mediaURLs []string,
) (repository.Review, error) {
	if listingID == "" {
		return repository.Review{}, ErrEmptyListing
	}
	if rating < 1 || rating > 5 {
		return repository.Review{}, ErrInvalidRating
	}

	review := repository.Review{
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
			s.logger.Warn("verify purchase failed; recording unverified review",
				slog.String("order_id", orderID), slog.Any("err", err))
		} else {
			review.VerifiedPurchase = verified
			review.SellerID = sellerID
		}
	}

	saved, err := s.repo.CreateReview(ctx, review)
	if err != nil {
		return repository.Review{}, fmt.Errorf("create review: %w", err)
	}
	return saved, nil
}

func (s *ReviewService) ListReviews(
	ctx context.Context,
	listingID string,
	ratingFilter int32,
	page, pageSize int,
) ([]repository.Review, int64, error) {
	if listingID == "" {
		return nil, 0, ErrEmptyListing
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize
	return s.repo.ListReviews(ctx, listingID, ratingFilter, offset, pageSize)
}

func (s *ReviewService) GetRatingSummary(ctx context.Context, listingID string) (repository.RatingSummary, error) {
	if listingID == "" {
		return repository.RatingSummary{}, ErrEmptyListing
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
func (s *ReviewService) GetShopRatingSummary(ctx context.Context, sellerID string) (repository.ShopRatingSummary, error) {
	if sellerID == "" {
		return repository.ShopRatingSummary{}, ErrEmptySeller
	}
	return s.repo.GetShopRatingSummary(ctx, sellerID)
}
