package store_test

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"
	"github.com/buidangphuc/team-domain/internal/store"
)

func newPool(ctx context.Context, t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := testkit.Schema(t, testkit.SharedPostgres(t))
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect pool: %v", err)
	}
	t.Cleanup(pool.Close)

	files, err := filepath.Glob(filepath.Join("..", "..", "migrations", "*.up.sql"))
	if err != nil || len(files) == 0 {
		t.Fatalf("find migrations: %v (n=%d)", err, len(files))
	}
	sort.Strings(files)
	for _, f := range files {
		sqlText, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		if _, err := pool.Exec(ctx, string(sqlText)); err != nil {
			t.Fatalf("apply %s: %v", filepath.Base(f), err)
		}
	}
	return pool
}

func newOutboxStore(t *testing.T, pool *pgxpool.Pool) *outbox.Store {
	t.Helper()
	s, err := outbox.NewStore(pool, "outbox_events")
	if err != nil {
		t.Fatalf("outbox store: %v", err)
	}
	return s
}

func newPGRepo(ctx context.Context, t *testing.T) (*store.PostgresListingRepository, *pgxpool.Pool) {
	t.Helper()
	pool := newPool(ctx, t)
	return store.NewPostgresListingRepository(pool), pool
}

func seedPGListing(ctx context.Context, t *testing.T, repo *store.PostgresListingRepository, id string, stock int32) {
	t.Helper()
	if _, err := repo.Create(ctx, store.Listing{
		ID: id, Title: id, Currency: "VND", Status: "published", Stock: stock,
	}); err != nil {
		t.Fatalf("seed listing %s: %v", id, err)
	}
}

func pgStock(ctx context.Context, t *testing.T, repo *store.PostgresListingRepository, id string) int32 {
	t.Helper()
	l, err := repo.Get(ctx, id)
	if err != nil {
		t.Fatalf("get %s: %v", id, err)
	}
	return l.Stock
}

func newID() string { return uuid.NewString() }
