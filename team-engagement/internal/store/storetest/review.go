package storetest

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/buidangphuc/team-engagement/internal/store"
)

type FakeReviewRepository struct {
	mu    sync.RWMutex
	data  []store.Review
	votes map[string]map[string]struct{} // reviewID -> set(userID)
}

func NewFakeReviewRepository() *FakeReviewRepository {
	return &FakeReviewRepository{
		data:  make([]store.Review, 0),
		votes: map[string]map[string]struct{}{},
	}
}

func (r *FakeReviewRepository) CreateReview(_ context.Context, review store.Review) (store.Review, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if review.ID == "" {
		review.ID = uuid.NewString()
	}
	review.CreatedAt = time.Now()
	if review.MediaURLs == nil {
		review.MediaURLs = []string{}
	}
	if review.VerifiedPurchase {
		for _, e := range r.data {
			if e.VerifiedPurchase && e.OrderID == review.OrderID && e.ListingID == review.ListingID {
				return store.Review{}, store.ErrDuplicateReview
			}
		}
	}
	r.data = append(r.data, review)
	return review, nil
}

func (r *FakeReviewRepository) ListReviews(_ context.Context, listingID string, ratingFilter int32, offset, limit int) ([]store.Review, int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var filtered []store.Review
	for i := len(r.data) - 1; i >= 0; i-- {
		rev := r.data[i]
		if rev.ListingID == listingID && rev.HiddenAt == nil {
			if ratingFilter == 0 || rev.Rating == ratingFilter {
				filtered = append(filtered, rev)
			}
		}
	}
	// Same order as Postgres: newest first, id as the tiebreak.
	sort.SliceStable(filtered, func(i, j int) bool {
		if !filtered[i].CreatedAt.Equal(filtered[j].CreatedAt) {
			return filtered[i].CreatedAt.After(filtered[j].CreatedAt)
		}
		return filtered[i].ID > filtered[j].ID
	})

	total := int64(len(filtered))
	if offset > len(filtered) {
		return []store.Review{}, total, nil
	}
	end := offset + limit
	if end > len(filtered) {
		end = len(filtered)
	}
	return filtered[offset:end], total, nil
}

func summarize(reviews []store.Review) (float64, int64, store.RatingBreakdown) {
	var breakdown store.RatingBreakdown
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

func (r *FakeReviewRepository) GetRatingSummary(_ context.Context, listingID string) (store.RatingSummary, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var matched []store.Review
	for _, rev := range r.data {
		if rev.ListingID == listingID && rev.VerifiedPurchase && rev.HiddenAt == nil {
			matched = append(matched, rev)
		}
	}
	avg, total, breakdown := summarize(matched)
	return store.RatingSummary{ListingID: listingID, AverageRating: avg, ReviewCount: total, Breakdown: breakdown}, nil
}

func (r *FakeReviewRepository) GetShopRatingSummary(_ context.Context, sellerID string) (store.ShopRatingSummary, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var matched []store.Review
	for _, rev := range r.data {
		if rev.SellerID == sellerID && rev.VerifiedPurchase && rev.HiddenAt == nil {
			matched = append(matched, rev)
		}
	}
	avg, total, breakdown := summarize(matched)
	return store.ShopRatingSummary{SellerID: sellerID, AverageRating: avg, ReviewCount: total, Breakdown: breakdown}, nil
}

func (r *FakeReviewRepository) MarkHelpful(_ context.Context, reviewID, userID string) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	idx := -1
	for i := range r.data {
		if r.data[i].ID == reviewID && r.data[i].HiddenAt == nil {
			idx = i
			break
		}
	}
	if idx == -1 {
		return 0, store.ErrReviewNotFound
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

func (r *FakeReviewRepository) GetReview(_ context.Context, id string) (store.Review, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, rev := range r.data {
		if rev.ID == id {
			return rev, nil
		}
	}
	return store.Review{}, store.ErrReviewNotFound
}

func (r *FakeReviewRepository) HideReview(_ context.Context, id, adminID, reason string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.data {
		if r.data[i].ID != id {
			continue
		}
		if r.data[i].HiddenAt != nil {
			return false, nil
		}
		now := time.Now()
		r.data[i].HiddenAt, r.data[i].HiddenBy, r.data[i].HideReason = &now, adminID, reason
		return true, nil
	}
	return false, store.ErrReviewNotFound
}

var _ store.ReviewRepository = (*FakeReviewRepository)(nil)
