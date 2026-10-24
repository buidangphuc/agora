package storetest

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/pagination"
	"github.com/buidangphuc/team-engagement/internal/store"
)

type (
	Stats   = store.Stats
	Loyalty = store.Loyalty
)

func dayOf(t time.Time) time.Time {
	y, m, d := t.UTC().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func clampPageSize(pageSize int32) int {
	return pagination.Clamp(int(pageSize), store.DefaultPageSize, store.MaxPageSize)
}

type FakeRepository struct {
	mu       sync.Mutex
	favs     map[string]map[string]struct{}  // userID -> set(listingID)
	views    map[string]int64                // listingID -> view count
	hist     map[string][]string             // userID -> listingIDs, most-recent-first
	follows  map[string]map[string]struct{}  // userID -> set(sellerID)
	sellerLs map[string]map[string]time.Time // sellerID -> listingID -> created_at
	loyalty  map[string]Loyalty              // userID -> loyalty snapshot
	checkins map[string]map[string]struct{}  // userID -> set(day "2006-01-02")
}

func NewFakeRepository() *FakeRepository {
	return &FakeRepository{
		favs:     map[string]map[string]struct{}{},
		views:    map[string]int64{},
		hist:     map[string][]string{},
		follows:  map[string]map[string]struct{}{},
		sellerLs: map[string]map[string]time.Time{},
		loyalty:  map[string]Loyalty{},
		checkins: map[string]map[string]struct{}{},
	}
}

func (r *FakeRepository) AddFavorite(_ context.Context, userID, listingID string) (bool, error) {
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

func (r *FakeRepository) RemoveFavorite(_ context.Context, userID, listingID string) (bool, error) {
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

func (r *FakeRepository) IsFavorite(_ context.Context, userID, listingID string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.favs[userID][listingID]
	return ok, nil
}

func (r *FakeRepository) ListFavorites(_ context.Context, userID, cursor string, pageSize int32) ([]string, string, int64, error) {
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

func (r *FakeRepository) RecordView(_ context.Context, userID, listingID string) (int64, error) {
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

func (r *FakeRepository) GetRecentlyViewed(_ context.Context, userID, cursor string, pageSize int32) ([]string, string, int64, error) {
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

func (r *FakeRepository) GetStats(_ context.Context, listingID string) (Stats, error) {
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

func (r *FakeRepository) Follow(_ context.Context, userID, sellerID string) (bool, error) {
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

func (r *FakeRepository) Unfollow(_ context.Context, userID, sellerID string) (bool, error) {
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

func (r *FakeRepository) IsFollowing(_ context.Context, userID, sellerID string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.follows[userID][sellerID]
	return ok, nil
}

func (r *FakeRepository) ListFollowedSellers(_ context.Context, userID, cursor string, pageSize int32) ([]string, string, int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	all := make([]string, 0, len(r.follows[userID]))
	for id := range r.follows[userID] {
		all = append(all, id)
	}
	ids, next := keysetPage(all, cursor, pageSize)
	return ids, next, int64(len(all)), nil
}

func (r *FakeRepository) ListFollowedListings(_ context.Context, userID, cursor string, pageSize int32) ([]string, string, int64, error) {
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
	curAt, curID, hasCur, err := store.ParseFeedCursor(cursor)
	if err != nil {
		return nil, "", 0, err
	}
	limit := clampPageSize(pageSize)
	ids := make([]string, 0, limit)
	var next string
	var lastAt time.Time
	for _, e := range all {
		if hasCur && !e.at.Before(curAt) && (!e.at.Equal(curAt) || e.id >= curID) {
			continue
		}
		if len(ids) == limit {
			next = store.FeedCursor(lastAt, ids[len(ids)-1])
			break
		}
		ids = append(ids, e.id)
		lastAt = e.at
	}
	return ids, next, int64(len(all)), nil
}

func (r *FakeRepository) IndexSellerListing(ctx context.Context, sellerID, listingID string) error {
	return r.UpsertSellerListing(ctx, sellerID, listingID, time.Now())
}

func (r *FakeRepository) UpsertSellerListing(_ context.Context, sellerID, listingID string, createdAt time.Time) error {
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

func (r *FakeRepository) ListingSeller(_ context.Context, listingID string) (string, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for sid, set := range r.sellerLs {
		if _, ok := set[listingID]; ok {
			return sid, true, nil
		}
	}
	return "", false, nil
}

func (r *FakeRepository) RemoveSellerListing(_ context.Context, listingID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, set := range r.sellerLs {
		delete(set, listingID)
	}
	return nil
}

func (r *FakeRepository) CheckIn(_ context.Context, userID string, day time.Time) (Loyalty, int64, error) {
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
		CoinBalance: acct.CoinBalance + store.CheckInCoins,
		LastCheckin: d,
	}
	r.loyalty[userID] = acct
	days[key] = struct{}{}
	return acct, store.CheckInCoins, nil
}

func (r *FakeRepository) GetLoyalty(_ context.Context, userID string) (Loyalty, error) {
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

var _ store.Repository = (*FakeRepository)(nil)
