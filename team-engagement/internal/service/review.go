package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"unicode/utf8"

	"github.com/buidangphuc/team-engagement/internal/store"
	"github.com/buidangphuc/team-engagement/internal/upstream"
)

var (
	ErrInvalidRating = errors.New("rating must be between 1 and 5")
	ErrEmptyListing  = errors.New("listing_id is required")
	ErrEmptyReviewID = errors.New("review_id is required")
	ErrEmptySeller   = errors.New("seller_id is required")

	ErrOrderUnavailable = errors.New("order verification unavailable")
	ErrOrderNotEligible = errors.New("order not eligible for review")
	ErrReasonTooLong    = errors.New("reason must be at most 500 characters")
	ErrAuditUnavailable = errors.New("audit unavailable")
)

// maxHideReasonRunes bounds the moderation reason after trimming.
const maxHideReasonRunes = 500

// AuditWriter is the slice of internal/upstream.AuditClient the service needs.
type AuditWriter interface {
	WriteHideEvent(ctx context.Context, actorID, reviewID, listingID, reason string) error
	WriteEvent(ctx context.Context, ev upstream.AuditEvent) error
}

// OrderVerifier consults team-order to confirm a buyer completed an order for a
// listing. Implemented by internal/upstream.OrderClient; a nil verifier makes
// CreateReview fail closed with ErrOrderUnavailable.
type OrderVerifier interface {
	VerifyPurchase(ctx context.Context, buyerID, listingID, orderID string) (verified bool, sellerID string, err error)
}

type ReviewService struct {
	repo     store.ReviewRepository
	verifier OrderVerifier
	audit    AuditWriter
	logger   *slog.Logger
}

// WithAudit wires the audit writer. Without one, HideReview fails closed.
func (s *ReviewService) WithAudit(w AuditWriter) *ReviewService {
	s.audit = w
	return s
}

func NewReviewService(repo store.ReviewRepository, verifier OrderVerifier, logger *slog.Logger) *ReviewService {
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
) (store.Review, error) {
	if listingID == "" {
		return store.Review{}, ErrEmptyListing
	}
	if rating < 1 || rating > 5 {
		return store.Review{}, ErrInvalidRating
	}

	if orderID == "" {
		return store.Review{}, ErrEmptyOrderID
	}
	if s.verifier == nil {
		return store.Review{}, ErrOrderUnavailable
	}
	verified, sellerID, err := s.verifier.VerifyPurchase(ctx, userID, listingID, orderID)
	if err != nil {
		s.logger.Warn("verify purchase failed", slog.String("order_id", orderID), slog.Any("err", err))
		return store.Review{}, fmt.Errorf("%w: %v", ErrOrderUnavailable, err)
	}
	if !verified {
		return store.Review{}, ErrOrderNotEligible
	}

	review := store.Review{
		ListingID:        listingID,
		UserID:           userID,
		UserName:         userName,
		OrderID:          orderID,
		Rating:           rating,
		Comment:          comment,
		MediaURLs:        mediaURLs,
		VerifiedPurchase: true,
		SellerID:         sellerID,
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

// HideReview hides a review after durably writing an audit event. Hiding an
// already-hidden review is a no-op. The reason is never logged.
func (s *ReviewService) HideReview(ctx context.Context, adminID, reviewID, reason string) error {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return ErrEmptyReason
	}
	if utf8.RuneCountInString(reason) > maxHideReasonRunes {
		return ErrReasonTooLong
	}
	if reviewID == "" {
		return ErrEmptyReviewID
	}
	review, err := s.repo.GetReview(ctx, reviewID)
	if err != nil {
		return err
	}
	if review.HiddenAt != nil {
		return nil
	}
	if s.audit == nil {
		return ErrAuditUnavailable
	}
	if err := s.audit.WriteHideEvent(ctx, adminID, reviewID, review.ListingID, reason); err != nil {
		return fmt.Errorf("%w: %v", ErrAuditUnavailable, err)
	}
	if _, err := s.repo.HideReview(ctx, reviewID, adminID, reason); err != nil {
		return err
	}
	return nil
}
