package store_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/buidangphuc/team-domain/internal/store"
)

func TestPageSizeClamp(t *testing.T) {
	repo, pool := newPGRepo(t)
	ctx := t.Context()
	for i := 0; i < 150; i++ {
		id := fmt.Sprintf("L-%04d", i)
		if _, err := repo.Create(ctx, store.Listing{
			ID: id, Title: id, Currency: "VND", Status: "published", Stock: 1, SellerID: "S1",
		}); err != nil {
			t.Fatalf("seed %s: %v", id, err)
		}
	}
	cases := []struct {
		size int32
		want int
	}{{0, 50}, {100, 100}, {101, 100}, {-5, 50}}

	for _, c := range cases {
		page, err := repo.List(ctx, "", c.size, "published")
		if err != nil {
			t.Fatalf("List size %d: %v", c.size, err)
		}
		if len(page.Items) != c.want {
			t.Errorf("List size %d returned %d items, want %d", c.size, len(page.Items), c.want)
		}
		mine, err := repo.ListByOwner(ctx, "S1", "", c.size)
		if err != nil {
			t.Fatalf("ListByOwner size %d: %v", c.size, err)
		}
		if len(mine.Items) != c.want {
			t.Errorf("ListByOwner size %d returned %d items, want %d", c.size, len(mine.Items), c.want)
		}
	}

	if _, err := pool.Exec(ctx,
		`UPDATE listings SET status = 'pending_review', submitted_at = now() WHERE seller_id = 'S1'`); err != nil {
		t.Fatalf("queue all: %v", err)
	}
	for _, c := range cases {
		page, err := repo.ListModerationQueue(ctx, "", c.size)
		if err != nil {
			t.Fatalf("ListModerationQueue size %d: %v", c.size, err)
		}
		if len(page.Items) != c.want {
			t.Errorf("ListModerationQueue size %d returned %d items, want %d", c.size, len(page.Items), c.want)
		}
	}
}

func ids(items []store.Listing) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it.ID)
	}
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestExistingCursorFormatsAccepted(t *testing.T) {
	repo, pool := newPGRepo(t)
	ctx := t.Context()

	for i := 1; i <= 5; i++ {
		seedPGListing(t, repo, fmt.Sprintf("L-%04d", i), 1)
	}
	page, err := repo.List(ctx, "L-0002", 2, "published")
	if err != nil {
		t.Fatalf("List with listing-id cursor: %v", err)
	}
	if got, want := ids(page.Items), []string{"L-0003", "L-0004"}; !equalStrings(got, want) {
		t.Fatalf("List after cursor L-0002 = %v, want %v", got, want)
	}
	if page.NextCursor != "L-0004" {
		t.Fatalf("List next cursor = %q, want the last listing id L-0004", page.NextCursor)
	}

	base := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	for _, q := range []struct {
		id string
		at time.Time
	}{
		{"q-01", base},
		{"q-02", base.Add(1500 * time.Millisecond)},
		{"q-03", base.Add(2 * time.Second)},
		{"q-04", base.Add(2 * time.Second)},
		{"q-05", base.Add(3 * time.Second)},
	} {
		seedStatus(t, repo, pool, q.id, "pending_review", q.at, nil)
	}

	const (
		cursorAfterQ02 = "2026-10-01T09:00:01.5Z|q-02"
		cursorAfterQ04 = "2026-10-01T09:00:02Z|q-04"
	)
	first, err := repo.ListModerationQueue(ctx, "", 2)
	if err != nil {
		t.Fatalf("queue first page: %v", err)
	}
	if got, want := ids(first.Items), []string{"q-01", "q-02"}; !equalStrings(got, want) {
		t.Fatalf("queue first page = %v, want %v", got, want)
	}
	if first.NextCursor != cursorAfterQ02 {
		t.Fatalf("queue cursor = %q, want the pre-migration format %q", first.NextCursor, cursorAfterQ02)
	}
	second, err := repo.ListModerationQueue(ctx, cursorAfterQ02, 2)
	if err != nil {
		t.Fatalf("queue with captured cursor: %v", err)
	}
	if got, want := ids(second.Items), []string{"q-03", "q-04"}; !equalStrings(got, want) {
		t.Fatalf("queue after captured cursor = %v, want %v", got, want)
	}
	if second.NextCursor != cursorAfterQ04 {
		t.Fatalf("queue cursor = %q, want %q", second.NextCursor, cursorAfterQ04)
	}
	third, err := repo.ListModerationQueue(ctx, cursorAfterQ04, 2)
	if err != nil {
		t.Fatalf("queue with second captured cursor: %v", err)
	}
	if got, want := ids(third.Items), []string{"q-05"}; !equalStrings(got, want) || third.NextCursor != "" {
		t.Fatalf("queue last page = %v next %q, want %v and no next cursor", got, third.NextCursor, want)
	}

	if _, err := repo.ListModerationQueue(ctx, "not-a-cursor", 2); !errors.Is(err, store.ErrInvalidCursor) {
		t.Fatalf("malformed queue cursor err = %v, want ErrInvalidCursor", err)
	}
}
