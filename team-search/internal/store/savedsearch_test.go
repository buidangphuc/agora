package store_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/migrate"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"
	"github.com/buidangphuc/team-search/internal/store"
	"github.com/buidangphuc/team-search/migrations"
)

func migratedDSN(t *testing.T) string {
	t.Helper()
	dsn := testkit.Schema(t, testkit.SharedPostgres(t))
	if err := migrate.Up(t.Context(), dsn, migrations.FS); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	return dsn
}

func openStore(ctx context.Context, t *testing.T, dsn string) (*store.PostgresSavedSearchRepository, *pgxpool.Pool) {
	t.Helper()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return store.NewPostgresSavedSearchRepository(pool), pool
}

func TestSavedSearchesPersistAcrossStores_ASavedSearchSurvivesARestart(t *testing.T) {
	ctx := t.Context()
	dsn := migratedDSN(t)

	first, pool := openStore(ctx, t, dsn)
	created, err := first.Create(ctx, store.SavedSearch{
		ID: "ss-1", UserID: "u1", Query: "red bike", FiltersJSON: `{"city":"hanoi"}`,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.ID != "ss-1" || created.UserID != "u1" || created.CreatedAt.IsZero() {
		t.Fatalf("created = %+v, want id ss-1, user u1, non-zero CreatedAt", created)
	}
	pool.Close()

	second, _ := openStore(ctx, t, dsn)
	got, err := second.Get(ctx, "ss-1", "u1")
	if err != nil {
		t.Fatalf("get after restart: %v", err)
	}
	if got.Query != "red bike" || got.UserID != "u1" || got.ID != "ss-1" {
		t.Errorf("got %+v, want query 'red bike' for u1", got)
	}
	if got.FiltersJSON == "" {
		t.Fatalf("filters empty after restart")
	}
	if got.FiltersJSON != `{"city": "hanoi"}` && got.FiltersJSON != `{"city":"hanoi"}` {
		t.Errorf("filters = %q, want the city=hanoi object", got.FiltersJSON)
	}
	if !got.CreatedAt.Equal(created.CreatedAt) {
		t.Errorf("CreatedAt = %v, want %v", got.CreatedAt, created.CreatedAt)
	}
}

func TestStoreScopesEveryCallByUser_AnotherUsersSavedSearchIsNotFound(t *testing.T) {
	ctx := t.Context()
	s, _ := openStore(ctx, t, migratedDSN(t))
	if _, err := s.Create(ctx, store.SavedSearch{ID: "a-1", UserID: "alice", Query: "q"}); err != nil {
		t.Fatalf("create alice: %v", err)
	}
	if _, err := s.Create(ctx, store.SavedSearch{ID: "b-1", UserID: "bob", Query: "q"}); err != nil {
		t.Fatalf("create bob: %v", err)
	}

	if _, err := s.Get(ctx, "a-1", "bob"); !errors.Is(err, store.ErrSavedSearchNotFound) {
		t.Errorf("get other user's id: err = %v, want ErrSavedSearchNotFound", err)
	}
	if err := s.Delete(ctx, "a-1", "bob"); !errors.Is(err, store.ErrSavedSearchNotFound) {
		t.Errorf("delete other user's id: err = %v, want ErrSavedSearchNotFound", err)
	}
	if _, err := s.Get(ctx, "a-1", "alice"); err != nil {
		t.Errorf("alice's row must survive bob's delete attempt: %v", err)
	}
	if _, err := s.Get(ctx, "missing", "alice"); !errors.Is(err, store.ErrSavedSearchNotFound) {
		t.Errorf("get unknown id: err = %v, want ErrSavedSearchNotFound", err)
	}

	items, total, err := s.List(ctx, "alice", 10, 0)
	if err != nil {
		t.Fatalf("list alice: %v", err)
	}
	if total != 1 || len(items) != 1 || items[0].ID != "a-1" {
		t.Errorf("alice list = %+v total=%d, want only a-1", items, total)
	}

	if err := s.Delete(ctx, "a-1", "alice"); err != nil {
		t.Fatalf("owner delete: %v", err)
	}
	if err := s.Delete(ctx, "a-1", "alice"); !errors.Is(err, store.ErrSavedSearchNotFound) {
		t.Errorf("second delete: err = %v, want ErrSavedSearchNotFound", err)
	}
}

func seedN(ctx context.Context, t *testing.T, s *store.PostgresSavedSearchRepository, user string, n int) {
	t.Helper()
	for i := 1; i <= n; i++ {
		if _, err := s.Create(ctx, store.SavedSearch{ID: fmt.Sprintf("s-%d", i), UserID: user, Query: fmt.Sprintf("q%d", i)}); err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
	}
}

func TestListNewestFirstAndTotal_SavedSearchesListNewestFirstWithATotal(t *testing.T) {
	ctx := t.Context()
	s, _ := openStore(ctx, t, migratedDSN(t))
	seedN(ctx, t, s, "u1", 5)

	items, total, err := s.List(ctx, "u1", 3, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 5 {
		t.Errorf("total = %d, want 5", total)
	}
	want := []string{"s-5", "s-4", "s-3"}
	if len(items) != len(want) {
		t.Fatalf("len = %d, want %d", len(items), len(want))
	}
	for i, id := range want {
		if items[i].ID != id {
			t.Errorf("items[%d] = %s, want %s", i, items[i].ID, id)
		}
	}

	next, total, err := s.List(ctx, "u1", 3, 3)
	if err != nil {
		t.Fatalf("list page 2: %v", err)
	}
	if total != 5 || len(next) != 2 || next[0].ID != "s-2" || next[1].ID != "s-1" {
		t.Errorf("page 2 = %+v total=%d, want s-2,s-1 total 5", next, total)
	}
}

func TestListPageBoundary_SavedSearchesListNewestFirstWithATotal(t *testing.T) {
	ctx := t.Context()
	s, _ := openStore(ctx, t, migratedDSN(t))
	seedN(ctx, t, s, "u1", 3)

	for _, offset := range []int{3, 100} {
		items, total, err := s.List(ctx, "u1", 10, offset)
		if err != nil {
			t.Fatalf("list offset %d: %v", offset, err)
		}
		if len(items) != 0 || total != 3 {
			t.Errorf("offset %d: items=%d total=%d, want 0 items and total 3", offset, len(items), total)
		}
	}

	items, total, err := s.List(ctx, "nobody", 10, 0)
	if err != nil || len(items) != 0 || total != 0 {
		t.Errorf("unknown user: items=%d total=%d err=%v, want empty/0/nil", len(items), total, err)
	}
}
