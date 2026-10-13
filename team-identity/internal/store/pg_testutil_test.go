package store

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/migrate"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/testkit"

	"github.com/buidangphuc/team-identity/migrations"
)

// testPool starts a disposable Postgres with every migration applied and returns
// a pool over it. Without Docker the test skips, or fails under
// GO_SDK_REQUIRE_DOCKER (set by `make check`).
func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()
	url := testkit.Postgres(t)
	if err := migrate.Up(ctx, url, migrations.Files); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}
