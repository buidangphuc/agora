package repository_test

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/team-domain/internal/repository"
)

// newPGRepo returns a PostgresListingRepository over a FRESH, throw-away schema
// with every migrations/*.up.sql applied in order, so the tests exercise the real
// SQL (including migration 0010's constraints). It skips — with the reason — when
// TEST_DATABASE_URL is not set, e.g.:
//
//	docker compose -f docker-compose.local.yaml up -d postgres-listing
//	TEST_DATABASE_URL=postgresql://listing_svc:listing_pass@localhost:5433/listing_db go test ./internal/repository/...
func newPGRepo(t *testing.T) (*repository.PostgresListingRepository, *pgxpool.Pool) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set: Postgres-backed reservation tests skipped (set it to run them)")
	}
	ctx := context.Background()

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
		_, _ = admin.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
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
	return repository.NewPostgresListingRepository(pool), pool
}

// seedPGListing inserts a base-stock listing.
func seedPGListing(t *testing.T, repo *repository.PostgresListingRepository, id string, stock int32) {
	t.Helper()
	if _, err := repo.Create(context.Background(), repository.Listing{
		ID: id, Title: id, Currency: "VND", Status: "published", Stock: stock,
	}); err != nil {
		t.Fatalf("seed listing %s: %v", id, err)
	}
}

func pgStock(t *testing.T, repo *repository.PostgresListingRepository, id string) int32 {
	t.Helper()
	l, err := repo.Get(context.Background(), id)
	if err != nil {
		t.Fatalf("get %s: %v", id, err)
	}
	return l.Stock
}
