package store_test

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/buidangphuc/team-engagement/internal/store"
	"github.com/buidangphuc/team-engagement/internal/store/storetest"
)

type pagingRepos struct {
	name        string
	repo        store.Repository
	disputes    store.DisputeRepository
	collections store.CollectionRepository
}

func pagingFixtures(t *testing.T) []pagingRepos {
	t.Helper()
	pool := testPool(t)
	ob := testOutbox(t, pool)
	return []pagingRepos{
		{
			name:        "postgres",
			repo:        store.NewPostgresRepository(pool, ob),
			disputes:    store.NewPostgresDisputeRepository(pool, ob),
			collections: store.NewPostgresCollectionRepository(pool),
		},
		{
			name:        "memory",
			repo:        storetest.NewFakeRepository(),
			disputes:    storetest.NewFakeDisputeRepository(),
			collections: storetest.NewFakeCollectionRepository(),
		},
	}
}

func seedFavourites(t *testing.T, repo store.Repository, user string, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		if _, err := repo.AddFavorite(callerCtx(), user, fmt.Sprintf("L%03d", i)); err != nil {
			t.Fatal(err)
		}
	}
}

func seedDisputes(t *testing.T, repo store.DisputeRepository, n int) {
	t.Helper()
	base := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	for i := 0; i < n; i++ {
		at := base.Add(time.Duration(i) * time.Second)
		d := store.Dispute{
			ID: fmt.Sprintf("d-%03d", i), OrderID: "o-1", ClaimantID: "buyer-1", DefendantID: "seller-1",
			Reason: "x", Status: store.DisputeStatusOpen, CreatedAt: at, UpdatedAt: at,
		}
		if _, err := repo.CreateDispute(callerCtx(), d); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPageSizeClampBoundary(t *testing.T) {
	for _, fx := range pagingFixtures(t) {
		t.Run(fx.name, func(t *testing.T) {
			ctx := callerCtx()
			seedFavourites(t, fx.repo, "buyer-1", 150)

			ids, next, total, err := fx.repo.ListFavorites(ctx, "buyer-1", "", 101)
			if err != nil || len(ids) != 100 || next == "" || total != 150 {
				t.Fatalf("page size 101: len=%d next=%q total=%d err=%v, want 100 items, a cursor and total 150", len(ids), next, total, err)
			}
			ids, next, _, err = fx.repo.ListFavorites(ctx, "buyer-1", "", 100)
			if err != nil || len(ids) != 100 || next == "" {
				t.Fatalf("page size 100: len=%d next=%q err=%v, want 100 items and a cursor", len(ids), next, err)
			}
			rest, next, _, err := fx.repo.ListFavorites(ctx, "buyer-1", next, 100)
			if err != nil || len(rest) != 50 || next != "" {
				t.Fatalf("second page: len=%d next=%q err=%v, want the remaining 50 and no cursor", len(rest), next, err)
			}
		})
	}
}

func TestDefaultPageSizesPerList(t *testing.T) {
	for _, fx := range pagingFixtures(t) {
		t.Run(fx.name, func(t *testing.T) {
			ctx := callerCtx()
			for i := 0; i < 150; i++ {
				if _, err := fx.repo.AddFavorite(ctx, "buyer-1", fmt.Sprintf("L%03d", i)); err != nil {
					t.Fatal(err)
				}
				if _, err := fx.repo.Follow(ctx, "buyer-1", fmt.Sprintf("S%03d", i)); err != nil {
					t.Fatal(err)
				}
				if _, err := fx.repo.RecordView(ctx, "buyer-1", fmt.Sprintf("V%03d", i)); err != nil {
					t.Fatal(err)
				}
			}
			col, err := fx.collections.CreateCollection(ctx, "buyer-1", "wish")
			if err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 150; i++ {
				if _, err := fx.collections.AddToCollection(ctx, "buyer-1", col.ID, fmt.Sprintf("C%03d", i)); err != nil {
					t.Fatal(err)
				}
			}
			seedDisputes(t, fx.disputes, 150)

			lists := map[string]func(size int32) (int, error){
				"ListFavorites": func(size int32) (int, error) {
					ids, _, _, err := fx.repo.ListFavorites(ctx, "buyer-1", "", size)
					return len(ids), err
				},
				"ListFollowedSellers": func(size int32) (int, error) {
					ids, _, _, err := fx.repo.ListFollowedSellers(ctx, "buyer-1", "", size)
					return len(ids), err
				},
				"GetRecentlyViewed": func(size int32) (int, error) {
					ids, _, _, err := fx.repo.GetRecentlyViewed(ctx, "buyer-1", "", size)
					return len(ids), err
				},
				"ListCollectionItems": func(size int32) (int, error) {
					ids, _, _, err := fx.collections.ListCollectionItems(ctx, "buyer-1", col.ID, "", size)
					return len(ids), err
				},
				"ListDisputes": func(size int32) (int, error) {
					ds, _, err := fx.disputes.ListByStatus(ctx, store.DisputeStatusOpen, int(size), "")
					return len(ds), err
				},
			}
			cases := []struct {
				list string
				size int32
				want int
			}{
				{"ListFavorites", 0, 50},
				{"ListFollowedSellers", 0, 50},
				{"GetRecentlyViewed", 0, 50},
				{"ListCollectionItems", 0, 50},
				{"ListFavorites", 101, 100},
				{"ListFollowedSellers", 101, 100},
				{"GetRecentlyViewed", 101, 100},
				{"ListCollectionItems", 101, 100},
				{"ListDisputes", 0, 20},
				{"ListDisputes", 101, 100},
			}
			for _, c := range cases {
				got, err := lists[c.list](c.size)
				if err != nil || got != c.want {
					t.Errorf("%s page size %d: got %d items err=%v, want %d", c.list, c.size, got, err, c.want)
				}
			}
		})
	}
}

func TestDisputeCursorFormatUnchanged(t *testing.T) {
	cursor := base64.RawURLEncoding.EncodeToString([]byte("1760000000000000|D7"))
	at := time.UnixMicro(1760000000000000).UTC()
	rows := []struct {
		id string
		at time.Time
	}{
		{"D6", at.Add(-time.Second)},
		{"D7", at},
		{"D8", at},
		{"D9", at.Add(time.Second)},
	}
	pool := testPool(t)
	ob := testOutbox(t, pool)
	repos := map[string]store.DisputeRepository{
		"postgres": store.NewPostgresDisputeRepository(pool, ob),
		"memory":   storetest.NewFakeDisputeRepository(),
	}
	for name, repo := range repos {
		t.Run(name, func(t *testing.T) {
			for _, r := range rows {
				d := store.Dispute{ID: r.id, OrderID: "o-" + r.id, ClaimantID: "buyer-1", DefendantID: "seller-1",
					Reason: "x", Status: store.DisputeStatusOpen, CreatedAt: r.at, UpdatedAt: r.at}
				if _, err := repo.CreateDispute(context.Background(), d); err != nil {
					t.Fatal(err)
				}
			}
			got, next, err := repo.ListByStatus(context.Background(), store.DisputeStatusOpen, 20, cursor)
			if err != nil {
				t.Fatalf("list with a pre-migration cursor: %v", err)
			}
			var ids []string
			for _, d := range got {
				ids = append(ids, d.ID)
			}
			if !reflect.DeepEqual(ids, []string{"D8", "D9"}) || next != "" {
				t.Fatalf("page after D7 = %v next=%q, want [D8 D9] and no cursor", ids, next)
			}

			page, next, err := repo.ListByStatus(context.Background(), store.DisputeStatusOpen, 1, "")
			if err != nil || len(page) != 1 || next == "" {
				t.Fatalf("first page len=%d next=%q err=%v", len(page), next, err)
			}
			raw, err := base64.RawURLEncoding.DecodeString(next)
			want := fmt.Sprintf("%d|%s", page[0].CreatedAt.UnixMicro(), page[0].ID)
			if err != nil || string(raw) != want {
				t.Fatalf("next cursor decodes to %q err=%v, want %q", raw, err, want)
			}
		})
	}
}

func TestFollowedListingsFeedCursorStaysInvalidOnGarbage(t *testing.T) {
	pool := testPool(t)
	repo := store.NewPostgresRepository(pool, testOutbox(t, pool))
	if _, _, _, err := repo.ListFollowedListings(callerCtx(), "buyer-1", "garbage", 10); !errors.Is(err, store.ErrInvalidCursor) {
		t.Fatalf("garbage feed cursor: err = %v, want ErrInvalidCursor", err)
	}
}
