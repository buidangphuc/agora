package repository

import (
	"context"
	"testing"
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
	for _, lid := range []string{"listing_1", "listing_2"} {
		if err := r.IndexSellerListing(ctx, "seller_1", lid); err != nil {
			t.Fatalf("index seller_1: %v", err)
		}
	}
	if err := r.IndexSellerListing(ctx, "seller_2", "listing_3"); err != nil {
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
	want := []string{"listing_1", "listing_2", "listing_3"}
	if len(ids) != len(want) {
		t.Fatalf("expected %v, got %v", want, ids)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("expected %v (sorted), got %v", want, ids)
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
