// Package pgtest gives Postgres-backed tests a fresh, throw-away schema with the repo's
// migrations applied, so they exercise the real SQL and constraints. The server is the
// SDK testkit's shared container (one per test binary); each pool gets its own schema,
// dropped on cleanup, so packages running in parallel never see each other's rows.
package pgtest

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/migrate"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/testkit"

	"github.com/buidangphuc/team-payment/migrations"
)

// MigrationsDir is the absolute path of team-payment's migrations directory.
func MigrationsDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "migrations")
}

// UpFiles returns the *.up.sql migrations in order.
func UpFiles(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(MigrationsDir(), "*.up.sql"))
	if err != nil || len(files) == 0 {
		t.Fatalf("find migrations: %v (n=%d)", err, len(files))
	}
	sort.Strings(files)
	return files
}

// Apply executes one migration file on the pool. Partial-apply tests use it; a test that
// applies every migration uses Pool, which goes through migrate.Up.
func Apply(t *testing.T, pool *pgxpool.Pool, file string) {
	t.Helper()
	sqlText, err := os.ReadFile(file) //nolint:gosec // file comes from the repo migrations glob
	if err != nil {
		t.Fatalf("read %s: %v", file, err)
	}
	if _, err := pool.Exec(context.Background(), string(sqlText)); err != nil {
		t.Fatalf("apply %s: %v", filepath.Base(file), err)
	}
}

// SchemaURL returns a DSN over a fresh schema of the shared Postgres, with no migrations
// applied. The schema search path ends in public, where the uuid-ossp extension lives.
func SchemaURL(t *testing.T) string {
	t.Helper()
	dsn := testkit.SharedPostgres(t)
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect admin pool: %v", err)
	}
	defer admin.Close()
	// 0001 creates uuid-ossp; create it once in public so a per-test schema neither owns
	// it (DROP SCHEMA CASCADE would drop it) nor races another package creating it.
	_, _ = admin.Exec(ctx, `CREATE EXTENSION IF NOT EXISTS "uuid-ossp" SCHEMA public`)

	schemaDSN := testkit.Schema(t, dsn)
	u, err := url.Parse(schemaDSN)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	q := u.Query()
	q.Set("search_path", q.Get("search_path")+",public")
	u.RawQuery = q.Encode()
	return u.String()
}

// EmptyPool returns a pool over a fresh schema with no migrations applied.
func EmptyPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), SchemaURL(t))
	if err != nil {
		t.Fatalf("connect pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// Pool returns a pool over a fresh schema with every migration applied.
func Pool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := SchemaURL(t)
	if err := migrate.Up(context.Background(), dsn, migrations.Files); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}
