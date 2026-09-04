package repository

import (
	"context"
	"testing"
)

// record→read roundtrip: an authenticated view shows up in the caller's history.
func TestRecentlyViewed_Roundtrip(t *testing.T) {
	r := NewInMemoryRepository()
	ctx := context.Background()

	if _, err := r.RecordView(ctx, "userA", "listing_1"); err != nil {
		t.Fatalf("record view: %v", err)
	}

	ids, next, total, err := r.GetRecentlyViewed(ctx, "userA", "", 10)
	if err != nil {
		t.Fatalf("get recently viewed: %v", err)
	}
	if len(ids) != 1 || ids[0] != "listing_1" {
		t.Fatalf("expected [listing_1], got %v", ids)
	}
	if total != 1 {
		t.Fatalf("expected total 1, got %d", total)
	}
	if next != "" {
		t.Fatalf("expected empty next cursor, got %q", next)
	}
}

// dedup bump: re-viewing a listing bumps it to the front without duplicating.
func TestRecentlyViewed_DedupBump(t *testing.T) {
	r := NewInMemoryRepository()
	ctx := context.Background()

	for _, id := range []string{"listing_1", "listing_2", "listing_1"} {
		if _, err := r.RecordView(ctx, "userA", id); err != nil {
			t.Fatalf("record view %s: %v", id, err)
		}
	}

	ids, _, total, err := r.GetRecentlyViewed(ctx, "userA", "", 10)
	if err != nil {
		t.Fatalf("get recently viewed: %v", err)
	}
	// listing_1 re-viewed last → front; no duplicate row.
	want := []string{"listing_1", "listing_2"}
	if len(ids) != len(want) {
		t.Fatalf("expected %v, got %v", want, ids)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("expected %v (newest-first), got %v", want, ids)
		}
	}
	if total != 2 {
		t.Fatalf("expected total 2 (deduped), got %d", total)
	}
}

// cross-user isolation: one user's history never leaks into another's.
func TestRecentlyViewed_CrossUserIsolation(t *testing.T) {
	r := NewInMemoryRepository()
	ctx := context.Background()

	if _, err := r.RecordView(ctx, "userA", "listing_a"); err != nil {
		t.Fatalf("record view A: %v", err)
	}
	if _, err := r.RecordView(ctx, "userB", "listing_b"); err != nil {
		t.Fatalf("record view B: %v", err)
	}

	aIDs, _, _, err := r.GetRecentlyViewed(ctx, "userA", "", 10)
	if err != nil {
		t.Fatalf("get A: %v", err)
	}
	if len(aIDs) != 1 || aIDs[0] != "listing_a" {
		t.Fatalf("userA expected [listing_a], got %v", aIDs)
	}

	bIDs, _, _, err := r.GetRecentlyViewed(ctx, "userB", "", 10)
	if err != nil {
		t.Fatalf("get B: %v", err)
	}
	if len(bIDs) != 1 || bIDs[0] != "listing_b" {
		t.Fatalf("userB expected [listing_b], got %v", bIDs)
	}
}

// anonymous: counter still increments but no history is recorded, and a read
// returns empty (no error).
func TestRecentlyViewed_Anonymous(t *testing.T) {
	r := NewInMemoryRepository()
	ctx := context.Background()

	count, err := r.RecordView(ctx, "", "listing_1")
	if err != nil {
		t.Fatalf("anonymous record view: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected view count 1, got %d", count)
	}

	ids, _, total, err := r.GetRecentlyViewed(ctx, "", "", 10)
	if err != nil {
		t.Fatalf("anonymous get: %v", err)
	}
	if len(ids) != 0 || total != 0 {
		t.Fatalf("anonymous expected empty history, got ids=%v total=%d", ids, total)
	}
}
