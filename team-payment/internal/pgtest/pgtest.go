// Package pgtest gives Postgres-backed tests a fresh, throw-away schema with the repo's
// migrations applied, so they exercise the real SQL and constraints. Tests skip, naming
// the reason, when TEST_DATABASE_URL is not set, e.g.:
//
//	docker run -d --rm --name pg-payment -e POSTGRES_PASSWORD=pg -p 127.0.0.1:55491:5432 postgres:16-alpine
//	TEST_DATABASE_URL=postgres://postgres:pg@127.0.0.1:55491/postgres?sslmode=disable go test ./...
//
// Each pool gets its own schema (dropped on cleanup), so packages running in parallel
// against one database never see each other's rows.
package pgtest

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
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

// Apply executes one migration file on the pool.
func Apply(t *testing.T, pool *pgxpool.Pool, file string) {
	t.Helper()
	sqlText, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("read %s: %v", file, err)
	}
	if _, err := pool.Exec(context.Background(), string(sqlText)); err != nil {
		t.Fatalf("apply %s: %v", filepath.Base(file), err)
	}
}

// EmptyPool returns a pool over a fresh schema with no migrations applied.
func EmptyPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set: Postgres-backed test skipped (set it to run)")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("connect admin pool: %v", err)
	}
	// 0001 creates uuid-ossp; create it once in public so a per-test schema neither owns
	// it (DROP SCHEMA CASCADE would drop it) nor races another package creating it.
	_, _ = admin.Exec(ctx, `CREATE EXTENSION IF NOT EXISTS "uuid-ossp" SCHEMA public`)
	schema := "t_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		admin.Close()
		t.Fatalf("create schema: %v", err)
	}
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("connect pool: %v", err)
	}
	t.Cleanup(func() {
		pool.Close()
		_, _ = admin.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
		admin.Close()
	})
	return pool
}

// Pool returns a pool over a fresh schema with every migration applied.
func Pool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool := EmptyPool(t)
	for _, f := range UpFiles(t) {
		Apply(t, pool, f)
	}
	return pool
}
