// Package repository is team-engagement's persistence boundary: per-user
// favorites and per-listing stats (view/favorite counts). Swappable store
// (Postgres in prod, in-memory for tests).
package repository

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// ErrInvalidCursor is returned for a malformed follow-feed cursor.
var ErrInvalidCursor = errors.New("invalid feed cursor")

const (
	DefaultPageSize = 50
	MaxPageSize     = 100
)

func clampPageSize(pageSize int32) int {
	switch {
	case pageSize <= 0:
		return DefaultPageSize
	case int(pageSize) > MaxPageSize:
		return MaxPageSize
	default:
		return int(pageSize)
	}
}

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
	// The cursor is opaque (see feedCursor).
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

// InMemoryRepository is a fake store for tests.
type InMemoryRepository struct {
	mu       sync.Mutex
	favs     map[string]map[string]struct{}  // userID -> set(listingID)
	views    map[string]int64                // listingID -> view count
	hist     map[string][]string             // userID -> listingIDs, most-recent-first
	follows  map[string]map[string]struct{}  // userID -> set(sellerID)
	sellerLs map[string]map[string]time.Time // sellerID -> listingID -> created_at
	loyalty  map[string]Loyalty              // userID -> loyalty snapshot
	checkins map[string]map[string]struct{}  // userID -> set(day "2006-01-02")
}

func NewInMemoryRepository() *InMemoryRepository {
	return &InMemoryRepository{
		favs:     map[string]map[string]struct{}{},
		views:    map[string]int64{},
		hist:     map[string][]string{},
		follows:  map[string]map[string]struct{}{},
		sellerLs: map[string]map[string]time.Time{},
		loyalty:  map[string]Loyalty{},
		checkins: map[string]map[string]struct{}{},
	}
}

func (r *InMemoryRepository) AddFavorite(_ context.Context, userID, listingID string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	set := r.favs[userID]
	if set == nil {
		set = map[string]struct{}{}
		r.favs[userID] = set
	}
	if _, ok := set[listingID]; ok {
		return false, nil
	}
	set[listingID] = struct{}{}
	return true, nil
}

func (r *InMemoryRepository) RemoveFavorite(_ context.Context, userID, listingID string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	set := r.favs[userID]
	if set == nil {
		return false, nil
	}
	if _, ok := set[listingID]; !ok {
		return false, nil
	}
	delete(set, listingID)
	return true, nil
}

func (r *InMemoryRepository) IsFavorite(_ context.Context, userID, listingID string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.favs[userID][listingID]
	return ok, nil
}

func (r *InMemoryRepository) ListFavorites(_ context.Context, userID, cursor string, pageSize int32) ([]string, string, int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	all := make([]string, 0, len(r.favs[userID]))
	for id := range r.favs[userID] {
		all = append(all, id)
	}
	sort.Strings(all)
	limit := clampPageSize(pageSize)
	ids := make([]string, 0, limit)
	var next string
	for _, id := range all {
		if cursor != "" && id <= cursor {
			continue
		}
		if len(ids) == limit {
			next = ids[len(ids)-1]
			break
		}
		ids = append(ids, id)
	}
	return ids, next, int64(len(all)), nil
}

func (r *InMemoryRepository) RecordView(_ context.Context, userID, listingID string) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.views[listingID]++
	if userID != "" {
		// Dedup then prepend so the freshest view sits at the front.
		cur := r.hist[userID]
		out := cur[:0:0]
		for _, id := range cur {
			if id != listingID {
				out = append(out, id)
			}
		}
		r.hist[userID] = append([]string{listingID}, out...)
	}
	return r.views[listingID], nil
}

func (r *InMemoryRepository) GetRecentlyViewed(_ context.Context, userID, cursor string, pageSize int32) ([]string, string, int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if userID == "" {
		return []string{}, "", 0, nil
	}
	all := r.hist[userID] // most-recent-first
	limit := clampPageSize(pageSize)
	start := 0
	if cursor != "" {
		for i, id := range all {
			if id == cursor {
				start = i + 1
				break
			}
		}
	}
	ids := make([]string, 0, limit)
	var next string
	for _, id := range all[start:] {
		if len(ids) == limit {
			next = ids[len(ids)-1]
			break
		}
		ids = append(ids, id)
	}
	return ids, next, int64(len(all)), nil
}

func (r *InMemoryRepository) GetStats(_ context.Context, listingID string) (Stats, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var fav int64
	for _, set := range r.favs {
		if _, ok := set[listingID]; ok {
			fav++
		}
	}
	return Stats{ViewCount: r.views[listingID], FavoriteCount: fav}, nil
}

func (r *InMemoryRepository) Follow(_ context.Context, userID, sellerID string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	set := r.follows[userID]
	if set == nil {
		set = map[string]struct{}{}
		r.follows[userID] = set
	}
	if _, ok := set[sellerID]; ok {
		return false, nil
	}
	set[sellerID] = struct{}{}
	return true, nil
}

func (r *InMemoryRepository) Unfollow(_ context.Context, userID, sellerID string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	set := r.follows[userID]
	if set == nil {
		return false, nil
	}
	if _, ok := set[sellerID]; !ok {
		return false, nil
	}
	delete(set, sellerID)
	return true, nil
}

func (r *InMemoryRepository) IsFollowing(_ context.Context, userID, sellerID string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.follows[userID][sellerID]
	return ok, nil
}

func (r *InMemoryRepository) ListFollowedSellers(_ context.Context, userID, cursor string, pageSize int32) ([]string, string, int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	all := make([]string, 0, len(r.follows[userID]))
	for id := range r.follows[userID] {
		all = append(all, id)
	}
	ids, next := keysetPage(all, cursor, pageSize)
	return ids, next, int64(len(all)), nil
}

func (r *InMemoryRepository) ListFollowedListings(_ context.Context, userID, cursor string, pageSize int32) ([]string, string, int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	type entry struct {
		id string
		at time.Time
	}
	var all []entry
	for sellerID := range r.follows[userID] {
		for lid, at := range r.sellerLs[sellerID] {
			all = append(all, entry{lid, at})
		}
	}
	// Newest first; listing id breaks ties so the order is total.
	sort.Slice(all, func(i, j int) bool {
		if !all[i].at.Equal(all[j].at) {
			return all[i].at.After(all[j].at)
		}
		return all[i].id > all[j].id
	})
	curAt, curID, hasCur, err := parseFeedCursor(cursor)
	if err != nil {
		return nil, "", 0, err
	}
	limit := clampPageSize(pageSize)
	ids := make([]string, 0, limit)
	var next string
	var lastAt time.Time
	for _, e := range all {
		if hasCur && !(e.at.Before(curAt) || (e.at.Equal(curAt) && e.id < curID)) {
			continue
		}
		if len(ids) == limit {
			next = feedCursor(lastAt, ids[len(ids)-1])
			break
		}
		ids = append(ids, e.id)
		lastAt = e.at
	}
	return ids, next, int64(len(all)), nil
}

func (r *InMemoryRepository) IndexSellerListing(ctx context.Context, sellerID, listingID string) error {
	return r.UpsertSellerListing(ctx, sellerID, listingID, time.Now())
}

func (r *InMemoryRepository) UpsertSellerListing(_ context.Context, sellerID, listingID string, createdAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	// A listing has exactly one owner: drop it from any other seller first,
	// keeping the original created_at when the owner is unchanged.
	prev, owned := r.sellerLs[sellerID][listingID]
	for sid, set := range r.sellerLs {
		if sid != sellerID {
			delete(set, listingID)
		}
	}
	set := r.sellerLs[sellerID]
	if set == nil {
		set = map[string]time.Time{}
		r.sellerLs[sellerID] = set
	}
	if owned {
		createdAt = prev
	}
	set[listingID] = createdAt.UTC()
	return nil
}

func (r *InMemoryRepository) ListingSeller(_ context.Context, listingID string) (string, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for sid, set := range r.sellerLs {
		if _, ok := set[listingID]; ok {
			return sid, true, nil
		}
	}
	return "", false, nil
}

func (r *InMemoryRepository) RemoveSellerListing(_ context.Context, listingID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, set := range r.sellerLs {
		delete(set, listingID)
	}
	return nil
}

func (r *InMemoryRepository) CheckIn(_ context.Context, userID string, day time.Time) (Loyalty, int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	d := dayOf(day)
	key := d.Format("2006-01-02")

	days := r.checkins[userID]
	if days == nil {
		days = map[string]struct{}{}
		r.checkins[userID] = days
	}
	acct := r.loyalty[userID]

	// Idempotent: already checked in today → return current snapshot, no reward.
	if _, ok := days[key]; ok {
		return acct, 0, nil
	}

	// Advance the streak, resetting to 1 unless the previous calendar day was
	// the last check-in (first-ever check-in also lands on 1).
	newStreak := int32(1)
	if !acct.LastCheckin.IsZero() && acct.LastCheckin.Equal(d.AddDate(0, 0, -1)) {
		newStreak = acct.Streak + 1
	}
	acct = Loyalty{
		Streak:      newStreak,
		CoinBalance: acct.CoinBalance + CheckInCoins,
		LastCheckin: d,
	}
	r.loyalty[userID] = acct
	days[key] = struct{}{}
	return acct, CheckInCoins, nil
}

func (r *InMemoryRepository) GetLoyalty(_ context.Context, userID string) (Loyalty, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.loyalty[userID], nil
}

// keysetPage sorts ids ascending and returns one clamped page after cursor,
// with the next cursor (empty when the page is the last). Mirrors the keyset
// pagination ListFavorites uses.
func keysetPage(all []string, cursor string, pageSize int32) ([]string, string) {
	sort.Strings(all)
	limit := clampPageSize(pageSize)
	ids := make([]string, 0, limit)
	var next string
	for _, id := range all {
		if cursor != "" && id <= cursor {
			continue
		}
		if len(ids) == limit {
			next = ids[len(ids)-1]
			break
		}
		ids = append(ids, id)
	}
	return ids, next
}

var _ Repository = (*InMemoryRepository)(nil)

// feedCursor encodes the keyset position of the follow feed: the last row's
// created_at and listing id, so paging stays stable while new listings arrive
// at the head.
func feedCursor(createdAt time.Time, listingID string) string {
	return createdAt.UTC().Format(time.RFC3339Nano) + "|" + listingID
}

// parseFeedCursor decodes a cursor produced by feedCursor. An empty cursor means
// "first page" (ok=false). A malformed cursor is an error rather than silently
// restarting the feed.
func parseFeedCursor(cursor string) (createdAt time.Time, listingID string, ok bool, err error) {
	if cursor == "" {
		return time.Time{}, "", false, nil
	}
	ts, id, found := strings.Cut(cursor, "|")
	if !found || id == "" {
		return time.Time{}, "", false, ErrInvalidCursor
	}
	t, perr := time.Parse(time.RFC3339Nano, ts)
	if perr != nil {
		return time.Time{}, "", false, fmt.Errorf("%w: %v", ErrInvalidCursor, perr)
	}
	return t, id, true, nil
}
