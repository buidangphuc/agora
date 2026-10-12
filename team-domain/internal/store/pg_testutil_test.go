package store_test

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/testkit"

	"github.com/buidangphuc/team-domain/internal/store"
)

// newPGRepo returns a PostgresListingRepository over a FRESH, throw-away schema
// with every migrations/*.up.sql applied in order, so the tests exercise the real
// SQL (including migration 0010's constraints). It uses TEST_DATABASE_URL when
// set, otherwise a testkit Postgres container, e.g.:
//
//	docker compose -f docker-compose.local.yaml up -d postgres-listing
//	TEST_DATABASE_URL=postgresql://listing_svc:listing_pass@localhost:5433/listing_db go test ./internal/store/...
func newPGRepo(ctx context.Context, t *testing.T) (*store.PostgresListingRepository, *pgxpool.Pool) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		// No database given: start a throw-away container (skips without Docker
		// unless GO_SDK_REQUIRE_DOCKER is set, as `make check` does).
		url = testkit.Postgres(t)
	}

	admin, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("connect admin pool: %v", err)
	}
	schema := "t_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		admin.Close()
		t.Fatalf("create schema: %v", err)
	}

	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("connect pool: %v", err)
	}
	t.Cleanup(func() {
		pool.Close()
		_, _ = admin.Exec(context.WithoutCancel(ctx), "DROP SCHEMA "+schema+" CASCADE")
		admin.Close()
	})

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
	return store.NewPostgresListingRepository(pool), pool
}

// seedPGListing inserts a base-stock listing.
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
