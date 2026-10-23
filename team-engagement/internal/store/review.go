package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/postgres"
	engagementv1 "github.com/buidangphuc/team-engagement/generated/platform/engagement/v1"
)

// ErrReviewNotFound is returned when a review id does not exist.
var ErrReviewNotFound = errors.New("review not found")

// ErrDuplicateReview is returned when a verified review already exists for the
// same (order, listing).
var ErrDuplicateReview = errors.New("review already exists for this order and listing")

type Review struct {
	ID               string
	ListingID        string
	UserID           string
	UserName         string
	OrderID          string
	Rating           int32
	Comment          string
	MediaURLs        []string
	HelpfulCount     int64
	VerifiedPurchase bool
	SellerID         string
	CreatedAt        time.Time
	HiddenAt         *time.Time
	HiddenBy         string
	HideReason       string
}

type RatingBreakdown struct {
	Star1 int32
	Star2 int32
	Star3 int32
	Star4 int32
	Star5 int32
}

type RatingSummary struct {
	ListingID     string
	AverageRating float64
	ReviewCount   int64
	Breakdown     RatingBreakdown
}

// ShopRatingSummary aggregates every review across a seller's listings (F4).
type ShopRatingSummary struct {
	SellerID      string
	AverageRating float64
	ReviewCount   int64
	Breakdown     RatingBreakdown
}

type ReviewRepository interface {
	CreateReview(ctx context.Context, r Review) (Review, error)
	ListReviews(ctx context.Context, listingID string, ratingFilter int32, offset, limit int) ([]Review, int64, error)
	GetRatingSummary(ctx context.Context, listingID string) (RatingSummary, error)
	// MarkHelpful is idempotent per (reviewID, userID); it returns the review's
	// current helpful_count. ErrReviewNotFound if the review does not exist.
	MarkHelpful(ctx context.Context, reviewID, userID string) (int64, error)
	// GetShopRatingSummary aggregates ratings across all of the seller's listings.
	GetShopRatingSummary(ctx context.Context, sellerID string) (ShopRatingSummary, error)
	// GetReview returns a review including hidden ones; ErrReviewNotFound if absent.
	GetReview(ctx context.Context, id string) (Review, error)
	// HideReview hides a review. Idempotent: changed=false when already hidden
	// (existing hidden_* state is kept). ErrReviewNotFound if absent.
	HideReview(ctx context.Context, id, adminID, reason string) (bool, error)
}

type PostgresReviewRepository struct {
	pool   *pgxpool.Pool
	outbox *outbox.Store
}

func NewPostgresReviewRepository(pool *pgxpool.Pool, ob *outbox.Store) *PostgresReviewRepository {
	return &PostgresReviewRepository{pool: pool, outbox: ob}
}

const reviewColumns = `id, listing_id, user_id, user_name, order_id, rating, comment, media_urls, helpful_count, verified_purchase, seller_id, created_at, hidden_at, hidden_by, hide_reason`

func scanReview(row pgx.Row, rev *Review) error {
	return row.Scan(&rev.ID, &rev.ListingID, &rev.UserID, &rev.UserName, &rev.OrderID,
		&rev.Rating, &rev.Comment, &rev.MediaURLs, &rev.HelpfulCount, &rev.VerifiedPurchase,
		&rev.SellerID, &rev.CreatedAt, &rev.HiddenAt, &rev.HiddenBy, &rev.HideReason)
}

func (r *PostgresReviewRepository) CreateReview(ctx context.Context, review Review) (Review, error) {
	if review.ID == "" {
		review.ID = uuid.NewString()
	}
	review.CreatedAt = time.Now()
	if review.MediaURLs == nil {
		review.MediaURLs = []string{}
	}

	const q = `INSERT INTO reviews (id, listing_id, user_id, user_name, order_id, rating, comment, media_urls, helpful_count, verified_purchase, seller_id, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`
	err := postgres.WithTx(ctx, r.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, q, review.ID, review.ListingID, review.UserID, review.UserName, review.OrderID,
			review.Rating, review.Comment, review.MediaURLs, review.HelpfulCount, review.VerifiedPurchase,
			review.SellerID, review.CreatedAt); err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "uq_reviews_order_listing_verified" {
				return ErrDuplicateReview
			}
			return fmt.Errorf("insert review: %w", err)
		}
		// Seller comes from the verified order; else from our own seller_listings
		// projection; empty when unknown. No review text travels in the fact.
		sellerID := review.SellerID
		if sellerID == "" {
			err := tx.QueryRow(ctx,
				`SELECT seller_id FROM seller_listings WHERE listing_id = $1 LIMIT 1`, review.ListingID).Scan(&sellerID)
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("lookup listing seller: %w", err)
			}
		}
		return enqueueFact(ctx, r.outbox, tx, "Listing", review.ListingID, &engagementv1.ReviewCreated{
			ReviewId: review.ID, UserId: review.UserID, ListingId: review.ListingID,
			SellerId: sellerID, Rating: review.Rating,
		})
	})
	if err != nil {
		return Review{}, err
	}
	return review, nil
}

func (r *PostgresReviewRepository) ListReviews(ctx context.Context, listingID string, ratingFilter int32, offset, limit int) ([]Review, int64, error) {
	var total int64
	var listQ string
	var rows pgx.Rows
	var err error

	if ratingFilter >= 1 && ratingFilter <= 5 {
		_ = r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM reviews WHERE listing_id = $1 AND hidden_at IS NULL AND rating = $2`, listingID, ratingFilter).Scan(&total)
		listQ = `SELECT ` + reviewColumns + ` FROM reviews WHERE listing_id = $1 AND hidden_at IS NULL AND rating = $2 ORDER BY created_at DESC, id DESC LIMIT $3 OFFSET $4`
		rows, err = r.pool.Query(ctx, listQ, listingID, ratingFilter, limit, offset)
	} else {
		_ = r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM reviews WHERE listing_id = $1 AND hidden_at IS NULL`, listingID).Scan(&total)
		listQ = `SELECT ` + reviewColumns + ` FROM reviews WHERE listing_id = $1 AND hidden_at IS NULL ORDER BY created_at DESC, id DESC LIMIT $2 OFFSET $3`
		rows, err = r.pool.Query(ctx, listQ, listingID, limit, offset)
	}

	if err != nil {
		return nil, 0, fmt.Errorf("query reviews: %w", err)
	}
	defer rows.Close()

	var reviews []Review
	for rows.Next() {
		var rev Review
		if err := scanReview(rows, &rev); err != nil {
			return nil, 0, err
		}
		reviews = append(reviews, rev)
	}
	return reviews, total, nil
}

func aggregateRows(rows pgx.Rows) (float64, int64, RatingBreakdown, error) {
	defer rows.Close()
	var breakdown RatingBreakdown
	var totalReviews, sumRatings int64
	for rows.Next() {
		var rating int32
		var count int64
		if err := rows.Scan(&rating, &count); err != nil {
			return 0, 0, RatingBreakdown{}, err
		}
		totalReviews += count
		sumRatings += int64(rating) * count
		switch rating {
		case 1:
			breakdown.Star1 = int32(count)
		case 2:
			breakdown.Star2 = int32(count)
		case 3:
			breakdown.Star3 = int32(count)
		case 4:
			breakdown.Star4 = int32(count)
		case 5:
			breakdown.Star5 = int32(count)
		}
	}
	var avg float64
	if totalReviews > 0 {
		avg = float64(sumRatings) / float64(totalReviews)
	}
	return avg, totalReviews, breakdown, rows.Err()
}

func (r *PostgresReviewRepository) GetRatingSummary(ctx context.Context, listingID string) (RatingSummary, error) {
	rows, err := r.pool.Query(ctx, `SELECT rating, COUNT(*) FROM reviews WHERE listing_id = $1 AND verified_purchase AND hidden_at IS NULL GROUP BY rating`, listingID)
	if err != nil {
		return RatingSummary{ListingID: listingID}, fmt.Errorf("query rating summary: %w", err)
	}
	avg, total, breakdown, err := aggregateRows(rows)
	if err != nil {
		return RatingSummary{ListingID: listingID}, err
	}
	return RatingSummary{ListingID: listingID, AverageRating: avg, ReviewCount: total, Breakdown: breakdown}, nil
}

func (r *PostgresReviewRepository) GetShopRatingSummary(ctx context.Context, sellerID string) (ShopRatingSummary, error) {
	rows, err := r.pool.Query(ctx, `SELECT rating, COUNT(*) FROM reviews WHERE seller_id = $1 AND verified_purchase AND hidden_at IS NULL GROUP BY rating`, sellerID)
	if err != nil {
		return ShopRatingSummary{SellerID: sellerID}, fmt.Errorf("query shop rating summary: %w", err)
	}
	avg, total, breakdown, err := aggregateRows(rows)
	if err != nil {
		return ShopRatingSummary{SellerID: sellerID}, err
	}
	return ShopRatingSummary{SellerID: sellerID, AverageRating: avg, ReviewCount: total, Breakdown: breakdown}, nil
}

func (r *PostgresReviewRepository) MarkHelpful(ctx context.Context, reviewID, userID string) (int64, error) {
	// Ensure the review exists first (FK would also reject, but this gives a clean error).
	var current int64
	err := r.pool.QueryRow(ctx, `SELECT helpful_count FROM reviews WHERE id = $1 AND hidden_at IS NULL`, reviewID).Scan(&current)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, ErrReviewNotFound
		}
		return 0, fmt.Errorf("load review: %w", err)
	}

	tag, err := r.pool.Exec(ctx,
		`INSERT INTO review_helpful_votes (review_id, user_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		reviewID, userID)
	if err != nil {
		return 0, fmt.Errorf("record helpful vote: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return current, nil // already voted — idempotent
	}

	var updated int64
	if err := r.pool.QueryRow(ctx,
		`UPDATE reviews SET helpful_count = helpful_count + 1 WHERE id = $1 RETURNING helpful_count`,
		reviewID).Scan(&updated); err != nil {
		return 0, fmt.Errorf("bump helpful_count: %w", err)
	}
	return updated, nil
}

func (r *PostgresReviewRepository) GetReview(ctx context.Context, id string) (Review, error) {
	var rev Review
	err := scanReview(r.pool.QueryRow(ctx, `SELECT `+reviewColumns+` FROM reviews WHERE id = $1`, id), &rev)
	if errors.Is(err, pgx.ErrNoRows) {
		return Review{}, ErrReviewNotFound
	}
	if err != nil {
		return Review{}, fmt.Errorf("get review: %w", err)
	}
	return rev, nil
}

func (r *PostgresReviewRepository) HideReview(ctx context.Context, id, adminID, reason string) (bool, error) {
	changed := false
	err := postgres.WithTx(ctx, r.pool, func(tx pgx.Tx) error {
		var listingID string
		var hiddenAt time.Time
		err := tx.QueryRow(ctx,
			`UPDATE reviews SET hidden_at = now(), hidden_by = $2, hide_reason = $3 WHERE id = $1 AND hidden_at IS NULL RETURNING listing_id, hidden_at`,
			id, adminID, reason).Scan(&listingID, &hiddenAt)
		if err == nil {
			changed = true
			// Moderator identity and reason stay out of the fact.
			return enqueueFact(ctx, r.outbox, tx, "Listing", listingID, &engagementv1.ReviewHidden{
				ReviewId: id, ListingId: listingID, HiddenAt: timestamppb.New(hiddenAt),
			})
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("hide review: %w", err)
		}
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM reviews WHERE id = $1)`, id).Scan(&exists); err != nil {
			return fmt.Errorf("hide review: %w", err)
		}
		if !exists {
			return ErrReviewNotFound
		}
		return nil
	})
	if err != nil {
		return false, err
	}
	return changed, nil
}

var _ ReviewRepository = (*PostgresReviewRepository)(nil)
