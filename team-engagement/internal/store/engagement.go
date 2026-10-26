package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrInvalidCursor is returned for a malformed follow-feed cursor.
var ErrInvalidCursor = errors.New("invalid feed cursor")

const (
	DefaultPageSize = 50
	MaxPageSize     = 100
)

// Stats are the per-listing engagement counters.
type Stats struct {
	ViewCount     int64
	FavoriteCount int64
}

// CheckInCoins is the coin reward granted by each successful daily check-in.
const CheckInCoins int64 = 10

// Loyalty is the per-user loyalty snapshot (streak, coin balance, and the
// calendar day of the last check-in — zero time when the user never checked in).
type Loyalty struct {
	Streak      int32
	CoinBalance int64
	LastCheckin time.Time
}

// dayOf truncates t to its UTC calendar date (midnight UTC).
func dayOf(t time.Time) time.Time {
	y, m, d := t.UTC().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// Repository is the port the handler depends on.
type Repository interface {
	// AddFavorite records (userID, listingID); added=false if it already existed.
	AddFavorite(ctx context.Context, userID, listingID string) (added bool, err error)
	// RemoveFavorite deletes it; removed=false if it wasn't there.
	RemoveFavorite(ctx context.Context, userID, listingID string) (removed bool, err error)
	IsFavorite(ctx context.Context, userID, listingID string) (bool, error)
	// ListFavorites returns the user's favorited listing ids, keyset by id.
	ListFavorites(ctx context.Context, userID, cursor string, pageSize int32) (ids []string, next string, total int64, err error)
	// RecordView increments and returns the listing's view count. For an
	// authenticated userID it also upserts the caller's recently-viewed
	// history (dedup on (userID, listingID), bumps viewed_at). An empty
	// userID (anonymous) only touches the counter.
	RecordView(ctx context.Context, userID, listingID string) (viewCount int64, err error)
	// GetRecentlyViewed returns the user's recently-viewed listing ids,
	// most-recent-first, cursor-paginated. Empty userID returns no rows.
	GetRecentlyViewed(ctx context.Context, userID, cursor string, pageSize int32) (ids []string, next string, total int64, err error)
	GetStats(ctx context.Context, listingID string) (Stats, error)

	// ── Seller follow graph (F1) ──

	// Follow records (userID, sellerID); added=false if it already existed.
	Follow(ctx context.Context, userID, sellerID string) (added bool, err error)
	// Unfollow deletes it; removed=false if it wasn't there.
	Unfollow(ctx context.Context, userID, sellerID string) (removed bool, err error)
	IsFollowing(ctx context.Context, userID, sellerID string) (bool, error)
	// ListFollowedSellers returns the user's followed seller ids, keyset by id.
	ListFollowedSellers(ctx context.Context, userID, cursor string, pageSize int32) (ids []string, next string, total int64, err error)
	// ListFollowedListings returns the listing ids belonging to sellers the user
	// follows (the follow feed), newest first (created_at DESC, listing id DESC).
	// The cursor is opaque (see FeedCursor).
	ListFollowedListings(ctx context.Context, userID, cursor string, pageSize int32) (ids []string, next string, total int64, err error)
	// IndexSellerListing records that listingID belongs to sellerID — the
	// feed source ListFollowedListings joins against. Idempotent. Shorthand for
	// UpsertSellerListing stamped with the current time.
	IndexSellerListing(ctx context.Context, sellerID, listingID string) error
	// UpsertSellerListing records a published listing for its seller. Idempotent:
	// a redelivered event keeps the original createdAt (the feed's sort key), and
	// a listing is only ever owned by one seller.
	UpsertSellerListing(ctx context.Context, sellerID, listingID string, createdAt time.Time) error
	// ListingSeller returns the seller that owns listingID per the seller_listings
	// projection (fed by listing.events). found=false when the projection has no
	// owner for it (not yet consumed, or removed).
	ListingSeller(ctx context.Context, listingID string) (sellerID string, found bool, err error)
	// RemoveSellerListing drops a deleted or unpublished listing from the feed
	// source. Idempotent; removing an unknown listing is a no-op.
	RemoveSellerListing(ctx context.Context, listingID string) error

	// ── Loyalty / daily check-in (F4) ──

	// CheckIn performs an idempotent daily check-in for the caller on the given
	// calendar day. The first check-in on a day advances the streak (reset to 1
	// if the previous day was skipped) and awards CheckInCoins; a repeat check-in
	// the same day is a no-op returning the current snapshot with coinsEarned=0.
	CheckIn(ctx context.Context, userID string, day time.Time) (loyalty Loyalty, coinsEarned int64, err error)
	// GetLoyalty returns the caller's loyalty snapshot (zero value if the user
	// has never checked in).
	GetLoyalty(ctx context.Context, userID string) (Loyalty, error)
}

// FeedCursor encodes the keyset position of the follow feed: the last row's
// created_at and listing id, so paging stays stable while new listings arrive
// at the head.
func FeedCursor(createdAt time.Time, listingID string) string {
	return createdAt.UTC().Format(time.RFC3339Nano) + "|" + listingID
}

// ParseFeedCursor decodes a cursor produced by FeedCursor. An empty cursor means
// "first page" (ok=false). A malformed cursor is an error rather than silently
// restarting the feed.
func ParseFeedCursor(cursor string) (createdAt time.Time, listingID string, ok bool, err error) {
	if cursor == "" {
		return time.Time{}, "", false, nil
	}
	ts, id, found := strings.Cut(cursor, "|")
	if !found || id == "" {
		return time.Time{}, "", false, ErrInvalidCursor
	}
	t, perr := time.Parse(time.RFC3339Nano, ts)
	if perr != nil {
		return time.Time{}, "", false, fmt.Errorf("%w: %w", ErrInvalidCursor, perr)
	}
	return t, id, true, nil
}
