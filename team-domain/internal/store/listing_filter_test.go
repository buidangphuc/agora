package store_test

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/buidangphuc/team-domain/internal/store"
	"github.com/buidangphuc/team-domain/internal/store/storetest"
)

type ownerRepo interface {
	Create(ctx context.Context, l store.Listing) (store.Listing, error)
	ListByOwner(ctx context.Context, ownerID string, filter store.ListingFilter, cursor string, pageSize int32) (store.Page, error)
}

func forEachRepo(t *testing.T, fn func(t *testing.T, repo ownerRepo)) {
	t.Run("postgres", func(t *testing.T) {
		repo, _ := newPGRepo(t.Context(), t)
		fn(t, repo)
	})
	t.Run("storetest", func(t *testing.T) {
		fn(t, storetest.NewInMemoryListingRepository(store.Listing{ID: "other", SellerID: "Z", Title: "x", Status: "published"}))
	})
}

func seedFilterListing(t *testing.T, repo ownerRepo, owner, id, title, status string) {
	t.Helper()
	if _, err := repo.Create(t.Context(), store.Listing{
		ID: id, Title: title, Currency: "VND", Status: status, Stock: 1, SellerID: owner,
	}); err != nil {
		t.Fatalf("seed %s: %v", id, err)
	}
}

func seedMixed(t *testing.T, repo ownerRepo) {
	t.Helper()
	for i, s := range []string{"published", "published", "published", "draft", "draft", "rejected"} {
		seedFilterListing(t, repo, "S1", fmt.Sprintf("m-%d", i+1), fmt.Sprintf("Item %d", i+1), s)
	}
	seedFilterListing(t, repo, "S2", "n-1", "Item other", "published")
}

func listIDs(t *testing.T, repo ownerRepo, owner string, f store.ListingFilter, cursor string, size int32) (store.Page, []string) {
	t.Helper()
	p, err := repo.ListByOwner(t.Context(), owner, f, cursor, size)
	if err != nil {
		t.Fatalf("ListByOwner: %v", err)
	}
	return p, ids(p.Items)
}

func wantIDs(t *testing.T, p store.Page, got, want []string) {
	t.Helper()
	if !equalStrings(got, want) || p.Total != int64(len(want)) {
		t.Fatalf("ids = %v total = %d, want %v total %d", got, p.Total, want, len(want))
	}
}

func TestListByOwnerFilterSingleStatus(t *testing.T) {
	forEachRepo(t, func(t *testing.T, repo ownerRepo) {
		seedMixed(t, repo)
		p, got := listIDs(t, repo, "S1", store.ListingFilter{Statuses: []string{"published"}}, "", 50)
		wantIDs(t, p, got, []string{"m-1", "m-2", "m-3"})
	})
}

func TestListByOwnerFilterSeveralStatusesAreOred(t *testing.T) {
	forEachRepo(t, func(t *testing.T, repo ownerRepo) {
		seedMixed(t, repo)
		p, got := listIDs(t, repo, "S1", store.ListingFilter{Statuses: []string{"draft", "rejected"}}, "", 50)
		wantIDs(t, p, got, []string{"m-4", "m-5", "m-6"})
	})
}

func TestListByOwnerFilterEmptyKeepsTodaysBehaviour(t *testing.T) {
	forEachRepo(t, func(t *testing.T, repo ownerRepo) {
		seedMixed(t, repo)
		p, got := listIDs(t, repo, "S1", store.ListingFilter{}, "", 50)
		wantIDs(t, p, got, []string{"m-1", "m-2", "m-3", "m-4", "m-5", "m-6"})
	})
}

func TestListByOwnerFilterDeletedStatusMatchesNothing(t *testing.T) {
	forEachRepo(t, func(t *testing.T, repo ownerRepo) {
		seedMixed(t, repo)
		p, got := listIDs(t, repo, "S1", store.ListingFilter{Statuses: []string{"deleted"}}, "", 50)
		wantIDs(t, p, got, nil)
		if p.NextCursor != "" {
			t.Errorf("next cursor = %q, want empty", p.NextCursor)
		}
	})
}

func TestListByOwnerFilterOtherSellerNeverReturned(t *testing.T) {
	forEachRepo(t, func(t *testing.T, repo ownerRepo) {
		seedMixed(t, repo)
		p, got := listIDs(t, repo, "S3", store.ListingFilter{Statuses: []string{"published"}}, "", 50)
		wantIDs(t, p, got, nil)
	})
}

func TestListByOwnerFilterQueryIgnoresCase(t *testing.T) {
	forEachRepo(t, func(t *testing.T, repo ownerRepo) {
		seedFilterListing(t, repo, "S1", "a-1", "Áo thun Đỏ", "published")
		seedFilterListing(t, repo, "S1", "a-2", "ÁO THUN xanh", "published")
		seedFilterListing(t, repo, "S1", "a-3", "Quần jean", "published")
		p, got := listIDs(t, repo, "S1", store.ListingFilter{Query: "áo thun"}, "", 50)
		wantIDs(t, p, got, []string{"a-1", "a-2"})
	})
}

func TestListByOwnerFilterQueryAndStatusCombine(t *testing.T) {
	forEachRepo(t, func(t *testing.T, repo ownerRepo) {
		seedFilterListing(t, repo, "S1", "c-1", "Áo thun", "published")
		seedFilterListing(t, repo, "S1", "c-2", "Áo thun", "draft")
		seedFilterListing(t, repo, "S1", "c-3", "Quần", "draft")
		p, got := listIDs(t, repo, "S1", store.ListingFilter{Statuses: []string{"draft"}, Query: "áo"}, "", 50)
		wantIDs(t, p, got, []string{"c-2"})
	})
}

func TestListByOwnerFilterLikeWildcardsMatchLiterally(t *testing.T) {
	forEachRepo(t, func(t *testing.T, repo ownerRepo) {
		seedFilterListing(t, repo, "S1", "w-1", "Giảm 50% áo", "published")
		seedFilterListing(t, repo, "S1", "w-2", "Giảm 500 áo", "published")
		seedFilterListing(t, repo, "S1", "w-3", "snake_case", "published")
		seedFilterListing(t, repo, "S1", "w-4", "snakeXcase", "published")
		seedFilterListing(t, repo, "S1", "w-5", `back\slash`, "published")
		seedFilterListing(t, repo, "S1", "w-6", `backXslash`, "published")
		for _, c := range []struct {
			q    string
			want []string
		}{
			{"50%", []string{"w-1"}},
			{"%", []string{"w-1"}},
			{"snake_case", []string{"w-3"}},
			{"_", []string{"w-3"}},
			{`back\slash`, []string{"w-5"}},
			{`\`, []string{"w-5"}},
		} {
			p, got := listIDs(t, repo, "S1", store.ListingFilter{Query: c.q}, "", 50)
			if !equalStrings(got, c.want) || p.Total != int64(len(c.want)) {
				t.Errorf("query %q = %v total %d, want %v", c.q, got, p.Total, c.want)
			}
		}
	})
}

func TestListByOwnerFilterPagesCoverFilteredSetExactlyOnce(t *testing.T) {
	forEachRepo(t, func(t *testing.T, repo ownerRepo) {
		for i := 0; i < 300; i++ {
			st := "published"
			if i%6 == 5 {
				st = "draft"
			}
			seedFilterListing(t, repo, "S1", fmt.Sprintf("p-%04d", i), "Item", st)
		}
		f := store.ListingFilter{Statuses: []string{"published"}}
		seen := map[string]bool{}
		var sizes []int
		cursor := ""
		for pages := 0; pages < 10; pages++ {
			p, got := listIDs(t, repo, "S1", f, cursor, 100)
			if p.Total != 250 {
				t.Errorf("page total = %d, want 250", p.Total)
			}
			sizes = append(sizes, len(got))
			for _, id := range got {
				if seen[id] {
					t.Fatalf("id %s repeated", id)
				}
				seen[id] = true
			}
			if !sort.StringsAreSorted(got) {
				t.Fatalf("page not ordered by id: %v", got)
			}
			if p.NextCursor == "" {
				break
			}
			cursor = p.NextCursor
		}
		if fmt.Sprint(sizes) != "[100 100 50]" {
			t.Fatalf("page sizes = %v, want [100 100 50]", sizes)
		}
		for id := range seen {
			var n int
			fmt.Sscanf(strings.TrimPrefix(id, "p-"), "%d", &n)
			if n%6 == 5 {
				t.Errorf("draft listing %s appeared in published pages", id)
			}
		}
	})
}

func TestListByOwnerFilterPageSizeClamped(t *testing.T) {
	forEachRepo(t, func(t *testing.T, repo ownerRepo) {
		for i := 0; i < 150; i++ {
			seedFilterListing(t, repo, "S1", fmt.Sprintf("k-%04d", i), "Item", "published")
		}
		p, got := listIDs(t, repo, "S1", store.ListingFilter{Statuses: []string{"published"}}, "", 1000)
		if len(got) != 100 || p.NextCursor == "" || p.Total != 150 {
			t.Fatalf("got %d items next %q total %d, want 100, non-empty, 150", len(got), p.NextCursor, p.Total)
		}
	})
}

func TestListByOwnerFilterForeignCursorYieldsOnlyOwnRowsAfterIt(t *testing.T) {
	forEachRepo(t, func(t *testing.T, repo ownerRepo) {
		seedMixed(t, repo)
		p, got := listIDs(t, repo, "S1", store.ListingFilter{Statuses: []string{"published"}}, "m-1", 50)
		if !equalStrings(got, []string{"m-2", "m-3"}) || p.Total != 3 {
			t.Fatalf("ids = %v total %d, want [m-2 m-3] total 3", got, p.Total)
		}
	})
}

func TestListByOwnerUnknownCursorIsInvalid(t *testing.T) {
	forEachRepo(t, func(t *testing.T, repo ownerRepo) {
		seedMixed(t, repo)
		for _, cursor := range []string{"not-a-cursor", "n-1"} {
			_, err := repo.ListByOwner(t.Context(), "S1", store.ListingFilter{}, cursor, 50)
			if !errors.Is(err, store.ErrInvalidCursor) {
				t.Fatalf("cursor %q: err = %v, want ErrInvalidCursor", cursor, err)
			}
		}
	})
}

func TestListByOwnerFilterLatencyAt2000Listings(t *testing.T) {
	repo, pool := newPGRepo(t.Context(), t)
	if _, err := pool.Exec(t.Context(), `INSERT INTO listings (id, title, price, currency, status, stock, seller_id)
		SELECT 'lat-' || lpad(g::text, 5, '0'),
			CASE WHEN g % 2 = 0 THEN 'Áo mẫu ' || g ELSE 'Quần mẫu ' || g END,
			0, 'VND', CASE WHEN g % 3 = 0 THEN 'draft' ELSE 'published' END, 1, 'S1'
		FROM generate_series(1, 2000) g`); err != nil {
		t.Fatalf("bulk seed: %v", err)
	}
	f := store.ListingFilter{Statuses: []string{"published"}, Query: "áo"}
	durs := make([]time.Duration, 0, 50)
	for i := 0; i < 50; i++ {
		start := time.Now()
		p, err := repo.ListByOwner(t.Context(), "S1", f, "", 20)
		durs = append(durs, time.Since(start))
		if err != nil {
			t.Fatalf("ListByOwner: %v", err)
		}
		if len(p.Items) != 20 || p.Total != 667 {
			t.Fatalf("got %d items total %d, want 20 and 667", len(p.Items), p.Total)
		}
	}
	sort.Slice(durs, func(i, j int) bool { return durs[i] < durs[j] })
	if p95 := durs[47]; p95 > 150*time.Millisecond {
		t.Fatalf("p95 = %v, want <= 150ms", p95)
	}
}
