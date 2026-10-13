package store_test

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/testkit"
)

// testDSN is the database a Postgres test runs against: TEST_DATABASE_URL when set
// (a disposable database), otherwise a fresh schema of the Postgres container the
// test binary shares (go-sdk testkit). Without Docker the test is skipped, or fails
// when GO_SDK_REQUIRE_DOCKER is set (make check).
func testDSN(t *testing.T) string {
	t.Helper()
	if dsn := os.Getenv("TEST_DATABASE_URL"); dsn != "" {
		return dsn
	}
	return testkit.Schema(t, testkit.SharedPostgres(t))
}

// pgPool opens the test database (see testDSN) with every migration applied.
func pgPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := testDSN(t)
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Skipf("cannot connect to Postgres (%v); skipping", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Skipf("Postgres unreachable (%v); skipping", err)
	}
	t.Cleanup(pool.Close)
	applyMigrations(ctx, t, pool)
	return pool
}

// uid returns a unique id with a readable prefix, so tests never collide across
// runs against the same database.
func uid(prefix string) string { return prefix + "-" + uuid.NewString() }
