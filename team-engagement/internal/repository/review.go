package repository

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrReviewNotFound is returned when a review id does not exist.
var ErrReviewNotFound = errors.New("review not found")

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
}

// ── Postgres Implementation ──────────────────────────────────────────

type PostgresReviewRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresReviewRepository(pool *pgxpool.Pool) *PostgresReviewRepository {
	return &PostgresReviewRepository{pool: pool}
}

const reviewColumns = `id, listing_id, user_id, user_name, order_id, rating, comment, media_urls, helpful_count, verified_purchase, seller_id, created_at`

func scanReview(row pgx.Row, rev *Review) error {
	return row.Scan(&rev.ID, &rev.ListingID, &rev.UserID, &rev.UserName, &rev.OrderID,
		&rev.Rating, &rev.Comment, &rev.MediaURLs, &rev.HelpfulCount, &rev.VerifiedPurchase,
		&rev.SellerID, &rev.CreatedAt)
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
	if _, err := r.pool.Exec(ctx, q, review.ID, review.ListingID, review.UserID, review.UserName, review.OrderID,
		review.Rating, review.Comment, review.MediaURLs, review.HelpfulCount, review.VerifiedPurchase,
		review.SellerID, review.CreatedAt); err != nil {
		return Review{}, fmt.Errorf("insert review: %w", err)
	}
	return review, nil
}

func (r *PostgresReviewRepository) ListReviews(ctx context.Context, listingID string, ratingFilter int32, offset, limit int) ([]Review, int64, error) {
	var total int64
	var listQ string
	var rows pgx.Rows
	var err error

	if ratingFilter >= 1 && ratingFilter <= 5 {
		_ = r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM reviews WHERE listing_id = $1 AND rating = $2`, listingID, ratingFilter).Scan(&total)
		listQ = `SELECT ` + reviewColumns + ` FROM reviews WHERE listing_id = $1 AND rating = $2 ORDER BY created_at DESC LIMIT $3 OFFSET $4`
		rows, err = r.pool.Query(ctx, listQ, listingID, ratingFilter, limit, offset)
	} else {
		_ = r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM reviews WHERE listing_id = $1`, listingID).Scan(&total)
		listQ = `SELECT ` + reviewColumns + ` FROM reviews WHERE listing_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`
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
	rows, err := r.pool.Query(ctx, `SELECT rating, COUNT(*) FROM reviews WHERE listing_id = $1 GROUP BY rating`, listingID)
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
	rows, err := r.pool.Query(ctx, `SELECT rating, COUNT(*) FROM reviews WHERE seller_id = $1 GROUP BY rating`, sellerID)
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
	err := r.pool.QueryRow(ctx, `SELECT helpful_count FROM reviews WHERE id = $1`, reviewID).Scan(&current)
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

var _ ReviewRepository = (*PostgresReviewRepository)(nil)

// ── InMemory Implementation ───────────────────────────────────────────

type InMemoryReviewRepository struct {
	mu    sync.RWMutex
	data  []Review
	votes map[string]map[string]struct{} // reviewID -> set(userID)
}

func NewInMemoryReviewRepository() *InMemoryReviewRepository {
	return &InMemoryReviewRepository{
		data:  make([]Review, 0),
		votes: map[string]map[string]struct{}{},
	}
}

func (r *InMemoryReviewRepository) CreateReview(_ context.Context, review Review) (Review, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if review.ID == "" {
		review.ID = uuid.NewString()
	}
	review.CreatedAt = time.Now()
	if review.MediaURLs == nil {
		review.MediaURLs = []string{}
	}
	r.data = append(r.data, review)
	return review, nil
}

func (r *InMemoryReviewRepository) ListReviews(_ context.Context, listingID string, ratingFilter int32, offset, limit int) ([]Review, int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var filtered []Review
	for i := len(r.data) - 1; i >= 0; i-- {
		rev := r.data[i]
		if rev.ListingID == listingID {
			if ratingFilter == 0 || rev.Rating == ratingFilter {
				filtered = append(filtered, rev)
			}
		}
	}

	total := int64(len(filtered))
	if offset > len(filtered) {
		return []Review{}, total, nil
	}
	end := offset + limit
	if end > len(filtered) {
		end = len(filtered)
	}
	return filtered[offset:end], total, nil
}

func summarize(reviews []Review) (float64, int64, RatingBreakdown) {
	var breakdown RatingBreakdown
	var totalReviews, sumRatings int64
	for _, rev := range reviews {
		totalReviews++
		sumRatings += int64(rev.Rating)
		switch rev.Rating {
		case 1:
			breakdown.Star1++
		case 2:
			breakdown.Star2++
		case 3:
			breakdown.Star3++
		case 4:
			breakdown.Star4++
		case 5:
			breakdown.Star5++
		}
	}
	var avg float64
	if totalReviews > 0 {
		avg = float64(sumRatings) / float64(totalReviews)
	}
	return avg, totalReviews, breakdown
}

func (r *InMemoryReviewRepository) GetRatingSummary(_ context.Context, listingID string) (RatingSummary, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var matched []Review
	for _, rev := range r.data {
		if rev.ListingID == listingID {
			matched = append(matched, rev)
		}
	}
	avg, total, breakdown := summarize(matched)
	return RatingSummary{ListingID: listingID, AverageRating: avg, ReviewCount: total, Breakdown: breakdown}, nil
}

func (r *InMemoryReviewRepository) GetShopRatingSummary(_ context.Context, sellerID string) (ShopRatingSummary, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var matched []Review
	for _, rev := range r.data {
		if rev.SellerID == sellerID {
			matched = append(matched, rev)
		}
	}
	avg, total, breakdown := summarize(matched)
	return ShopRatingSummary{SellerID: sellerID, AverageRating: avg, ReviewCount: total, Breakdown: breakdown}, nil
}

func (r *InMemoryReviewRepository) MarkHelpful(_ context.Context, reviewID, userID string) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	idx := -1
	for i := range r.data {
		if r.data[i].ID == reviewID {
			idx = i
			break
		}
	}
	if idx == -1 {
		return 0, ErrReviewNotFound
	}

	set := r.votes[reviewID]
	if set == nil {
		set = map[string]struct{}{}
		r.votes[reviewID] = set
	}
	if _, ok := set[userID]; ok {
		return r.data[idx].HelpfulCount, nil // already voted — idempotent
	}
	set[userID] = struct{}{}
	r.data[idx].HelpfulCount++
	return r.data[idx].HelpfulCount, nil
}

var _ ReviewRepository = (*InMemoryReviewRepository)(nil)
