package repository

import (
	"context"
	"errors"
	"testing"
	"time"
)

// follow→list roundtrip: following a seller shows up in IsFollowing and the
// followed-sellers list.
func TestFollows_Roundtrip(t *testing.T) {
	r := NewInMemoryRepository()
	ctx := context.Background()

	added, err := r.Follow(ctx, "userA", "seller_1")
	if err != nil {
		t.Fatalf("follow: %v", err)
	}
	if !added {
		t.Fatalf("expected added=true on first follow")
	}

	// Idempotent: a repeat follow is a no-op (added=false), no duplicate row.
	added, err = r.Follow(ctx, "userA", "seller_1")
	if err != nil {
		t.Fatalf("re-follow: %v", err)
	}
	if added {
		t.Fatalf("expected added=false on duplicate follow")
	}

	following, err := r.IsFollowing(ctx, "userA", "seller_1")
	if err != nil {
		t.Fatalf("is following: %v", err)
	}
	if !following {
		t.Fatalf("expected IsFollowing=true")
	}

	ids, next, total, err := r.ListFollowedSellers(ctx, "userA", "", 10)
	if err != nil {
		t.Fatalf("list followed sellers: %v", err)
	}
	if len(ids) != 1 || ids[0] != "seller_1" {
		t.Fatalf("expected [seller_1], got %v", ids)
	}
	if total != 1 {
		t.Fatalf("expected total 1, got %d", total)
	}
	if next != "" {
		t.Fatalf("expected empty next cursor, got %q", next)
	}
}

// unfollow: removes the edge; IsFollowing flips to false and the list empties.
func TestFollows_Unfollow(t *testing.T) {
	r := NewInMemoryRepository()
	ctx := context.Background()

	if _, err := r.Follow(ctx, "userA", "seller_1"); err != nil {
		t.Fatalf("follow: %v", err)
	}

	removed, err := r.Unfollow(ctx, "userA", "seller_1")
	if err != nil {
		t.Fatalf("unfollow: %v", err)
	}
	if !removed {
		t.Fatalf("expected removed=true")
	}

	// Unfollowing again is a no-op.
	removed, err = r.Unfollow(ctx, "userA", "seller_1")
	if err != nil {
		t.Fatalf("re-unfollow: %v", err)
	}
	if removed {
		t.Fatalf("expected removed=false on second unfollow")
	}

	following, _ := r.IsFollowing(ctx, "userA", "seller_1")
	if following {
		t.Fatalf("expected IsFollowing=false after unfollow")
	}

	ids, _, total, err := r.ListFollowedSellers(ctx, "userA", "", 10)
	if err != nil {
		t.Fatalf("list followed sellers: %v", err)
	}
	if len(ids) != 0 || total != 0 {
		t.Fatalf("expected empty list, got ids=%v total=%d", ids, total)
	}
}

// cross-user isolation: one user's follow graph never leaks into another's.
func TestFollows_CrossUserIsolation(t *testing.T) {
	r := NewInMemoryRepository()
	ctx := context.Background()

	if _, err := r.Follow(ctx, "userA", "seller_1"); err != nil {
		t.Fatalf("follow A: %v", err)
	}
	if _, err := r.Follow(ctx, "userB", "seller_2"); err != nil {
		t.Fatalf("follow B: %v", err)
	}

	aFollowsB, _ := r.IsFollowing(ctx, "userA", "seller_2")
	if aFollowsB {
		t.Fatalf("userA must not appear to follow seller_2")
	}

	aIDs, _, _, err := r.ListFollowedSellers(ctx, "userA", "", 10)
	if err != nil {
		t.Fatalf("list A: %v", err)
	}
	if len(aIDs) != 1 || aIDs[0] != "seller_1" {
		t.Fatalf("userA expected [seller_1], got %v", aIDs)
	}

	bIDs, _, _, err := r.ListFollowedSellers(ctx, "userB", "", 10)
	if err != nil {
		t.Fatalf("list B: %v", err)
	}
	if len(bIDs) != 1 || bIDs[0] != "seller_2" {
		t.Fatalf("userB expected [seller_2], got %v", bIDs)
	}
}

// follow feed: ListFollowedListings returns the deduped union of listings from
// every followed seller, and only from followed sellers.
func TestFollows_FollowedListingsFeed(t *testing.T) {
	r := NewInMemoryRepository()
	ctx := context.Background()

	// Seed the seller→listing index (populated by listing events in prod).
	base := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	for i, lid := range []string{"listing_1", "listing_2"} {
		if err := r.UpsertSellerListing(ctx, "seller_1", lid, base.Add(time.Duration(i)*time.Minute)); err != nil {
			t.Fatalf("index seller_1: %v", err)
		}
	}
	if err := r.UpsertSellerListing(ctx, "seller_2", "listing_3", base.Add(2*time.Minute)); err != nil {
		t.Fatalf("index seller_2: %v", err)
	}
	// listing owned by a seller the user does NOT follow.
	if err := r.IndexSellerListing(ctx, "seller_other", "listing_9"); err != nil {
		t.Fatalf("index seller_other: %v", err)
	}

	if _, err := r.Follow(ctx, "userA", "seller_1"); err != nil {
		t.Fatalf("follow seller_1: %v", err)
	}
	if _, err := r.Follow(ctx, "userA", "seller_2"); err != nil {
		t.Fatalf("follow seller_2: %v", err)
	}

	ids, _, total, err := r.ListFollowedListings(ctx, "userA", "", 10)
	if err != nil {
		t.Fatalf("list followed listings: %v", err)
	}
	want := []string{"listing_3", "listing_2", "listing_1"} // newest first
	if len(ids) != len(want) {
		t.Fatalf("expected %v, got %v", want, ids)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("expected %v, got %v", want, ids)
		}
	}
	if total != 3 {
		t.Fatalf("expected total 3, got %d", total)
	}

	// An unfollowed user has an empty feed (no panic, no leak).
	empty, _, emptyTotal, err := r.ListFollowedListings(ctx, "userNobody", "", 10)
	if err != nil {
		t.Fatalf("list empty feed: %v", err)
	}
	if len(empty) != 0 || emptyTotal != 0 {
		t.Fatalf("expected empty feed, got ids=%v total=%d", empty, emptyTotal)
	}
}

// feed ordering: newest first (created_at DESC, listing id DESC on ties), with
// stable keyset paging across pages.
func TestFollows_FeedNewestFirstAndPaging(t *testing.T) {
	r := NewInMemoryRepository()
	ctx := context.Background()
	base := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)

	// Insert in an order that differs from both id order and time order.
	for _, e := range []struct {
		seller, id string
		at         time.Time
	}{
		{"s1", "a_old", base},
		{"s2", "z_mid", base.Add(time.Hour)},
		{"s1", "m_new", base.Add(2 * time.Hour)},
		{"s2", "b_tie", base.Add(time.Hour)}, // same instant as z_mid
		{"s3", "unfollowed", base.Add(3 * time.Hour)},
	} {
		if err := r.UpsertSellerListing(ctx, e.seller, e.id, e.at); err != nil {
			t.Fatal(err)
		}
	}
	for _, s := range []string{"s1", "s2"} {
		if _, err := r.Follow(ctx, "u", s); err != nil {
			t.Fatal(err)
		}
	}

	ids, next, total, err := r.ListFollowedListings(ctx, "u", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"m_new", "z_mid", "b_tie", "a_old"}
	if len(ids) != len(want) || total != 4 || next != "" {
		t.Fatalf("ids=%v total=%d next=%q, want %v", ids, total, next, want)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("ids=%v, want %v", ids, want)
		}
	}

	// Page through two at a time; a new listing arriving at the head between
	// pages must not shift or repeat rows.
	p1, next, _, err := r.ListFollowedListings(ctx, "u", "", 2)
	if err != nil || len(p1) != 2 || next == "" {
		t.Fatalf("page1=%v next=%q err=%v", p1, next, err)
	}
	if err := r.UpsertSellerListing(ctx, "s1", "brand_new", base.Add(5*time.Hour)); err != nil {
		t.Fatal(err)
	}
	p2, next2, _, err := r.ListFollowedListings(ctx, "u", next, 2)
	if err != nil || len(p2) != 2 || next2 != "" {
		t.Fatalf("page2=%v next=%q err=%v", p2, next2, err)
	}
	got := append(append([]string{}, p1...), p2...)
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("paged=%v, want %v", got, want)
		}
	}

	if _, _, _, err := r.ListFollowedListings(ctx, "u", "garbage", 2); !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("garbage cursor: err=%v, want ErrInvalidCursor", err)
	}
}

// UpsertSellerListing keeps the first created_at and a single owner; Remove is
// idempotent.
func TestFollows_UpsertKeepsCreatedAtAndSingleOwner(t *testing.T) {
	r := NewInMemoryRepository()
	ctx := context.Background()
	t1 := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	_ = r.UpsertSellerListing(ctx, "s1", "l", t1)
	_ = r.UpsertSellerListing(ctx, "s1", "l", t1.Add(time.Hour)) // redelivery/update
	if got := r.sellerLs["s1"]["l"]; !got.Equal(t1) {
		t.Fatalf("created_at = %v, want %v", got, t1)
	}
	_ = r.UpsertSellerListing(ctx, "s2", "l", t1) // ownership moved
	if _, ok := r.sellerLs["s1"]["l"]; ok {
		t.Fatal("listing must belong to one seller only")
	}
	_ = r.RemoveSellerListing(ctx, "l")
	if err := r.RemoveSellerListing(ctx, "l"); err != nil {
		t.Fatal(err)
	}
	if len(r.sellerLs["s2"]) != 0 {
		t.Fatal("listing must be removed")
	}
}
