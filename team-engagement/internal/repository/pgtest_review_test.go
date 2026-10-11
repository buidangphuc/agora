package repository

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// Reviews created in the same instant must still page deterministically: the
// id is the tiebreak behind created_at DESC.
func TestPostgresListReviewsStableOrderOnTimestampTies(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	ts := time.Now().Truncate(time.Microsecond)
	for i := 1; i <= 25; i++ {
		_, err := pool.Exec(ctx, `INSERT INTO reviews (id, listing_id, user_id, user_name, order_id, rating, comment, created_at)
			VALUES ($1,'l1','u','U','',5,'',$2)`, fmt.Sprintf("id-%02d", i), ts)
		if err != nil {
			t.Fatal(err)
		}
	}
	repo := NewPostgresReviewRepository(pool)
	var seen []string
	for off := 0; off < 25; off += 10 {
		revs, total, err := repo.ListReviews(ctx, "l1", 0, off, 10)
		if err != nil || total != 25 {
			t.Fatalf("offset %d: total=%d err=%v", off, total, err)
		}
		for _, r := range revs {
			seen = append(seen, r.ID)
		}
	}
	if len(seen) != 25 {
		t.Fatalf("saw %d reviews", len(seen))
	}
	for i, id := range seen {
		if want := fmt.Sprintf("id-%02d", 25-i); id != want {
			t.Fatalf("position %d = %s, want %s", i, id, want)
		}
	}
}
