package store_test

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/migrate"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/testkit"

	"github.com/buidangphuc/team-domain/internal/store"
	"github.com/buidangphuc/team-domain/migrations"
)

// newPGRepo returns a PostgresListingRepository over a FRESH, throw-away schema
// with every migration applied (migrate.Up over the embedded migrations), so the
// tests exercise the real SQL (including migration 0010's constraints). It uses
// TEST_DATABASE_URL when set, otherwise the test binary's shared testkit Postgres
// container, e.g.:
//
//	docker compose -f docker-compose.local.yaml up -d postgres-listing
//	TEST_DATABASE_URL=postgresql://listing_svc:listing_pass@localhost:5433/listing_db go test ./internal/store/...
func newPGRepo(ctx context.Context, t *testing.T) (*store.PostgresListingRepository, *pgxpool.Pool) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		// No database given: use the shared container (skips without Docker
		// unless GO_SDK_REQUIRE_DOCKER is set, as `make check` does).
		url = testkit.SharedPostgres(t)
	}
	dsn := testkit.Schema(t, url)
	if err := migrate.Up(ctx, dsn, migrations.Files); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return store.NewPostgresListingRepository(pool), pool
}

// newOutboxStore returns the SDK outbox store over the service's outbox table.
func newOutboxStore(t *testing.T, pool *pgxpool.Pool) *outbox.Store {
	t.Helper()
	s, err := outbox.NewStore(pool, store.OutboxTable)
	if err != nil {
		t.Fatalf("outbox store: %v", err)
	}
	return s
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
