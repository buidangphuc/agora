package repository_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// pgPool returns a pool bound to a throwaway schema, or skips when
// TEST_DATABASE_URL is unset. The schema is dropped on cleanup.
func pgPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	schema := fmt.Sprintf("t_audit_%d", time.Now().UnixNano())

	admin, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		admin.Close()
		t.Fatalf("create schema: %v", err)
	}
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(func() {
		pool.Close()
		_, _ = admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
		admin.Close()
	})
	return pool
}

func applyMigration(t *testing.T, pool *pgxpool.Pool, pattern string) {
	t.Helper()
	files, err := filepath.Glob(filepath.Join("..", "..", "migrations", pattern))
	if err != nil || len(files) == 0 {
		t.Fatalf("migration %q not found (err=%v)", pattern, err)
	}
	sort.Strings(files)
	for _, f := range files {
		sql, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		if _, err := pool.Exec(context.Background(), string(sql)); err != nil {
			t.Fatalf("apply %s: %v", f, err)
		}
	}
}

func actionIndexCount(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var n int
	err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM pg_indexes
		 WHERE schemaname = current_schema() AND indexname = 'idx_audit_events_action_created'`).Scan(&n)
	if err != nil {
		t.Fatalf("count index: %v", err)
	}
	return n
}

// Scenarios: Migration creates and drops the index; Migration up is re-runnable
// without failure.
func TestMigration002ActionIndex(t *testing.T) {
	pool := pgPool(t)
	ctx := context.Background()
	applyMigration(t, pool, "001_*.up.sql")
	if _, err := pool.Exec(ctx,
		`INSERT INTO audit_events (id, action) VALUES ('keep-1', 'a.b')`); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if n := actionIndexCount(t, pool); n != 0 {
		t.Fatalf("index must not exist before 002, got %d", n)
	}

	applyMigration(t, pool, "002_*.up.sql")
	if n := actionIndexCount(t, pool); n != 1 {
		t.Fatalf("after up: want 1 index, got %d", n)
	}
	var def string
	if err := pool.QueryRow(ctx,
		`SELECT indexdef FROM pg_indexes WHERE schemaname = current_schema()
		 AND indexname = 'idx_audit_events_action_created'`).Scan(&def); err != nil {
		t.Fatalf("indexdef: %v", err)
	}
	for _, want := range []string{"audit_events", "action", "created_at DESC"} {
		if !contains(def, want) {
			t.Fatalf("indexdef %q lacks %q", def, want)
		}
	}

	applyMigration(t, pool, "002_*.up.sql") // re-run must not fail
	if n := actionIndexCount(t, pool); n != 1 {
		t.Fatalf("after second up: want exactly 1 index, got %d", n)
	}

	applyMigration(t, pool, "002_*.down.sql")
	if n := actionIndexCount(t, pool); n != 0 {
		t.Fatalf("after down: want 0 indexes, got %d", n)
	}
	var rows int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_events`).Scan(&rows); err != nil || rows != 1 {
		t.Fatalf("existing rows must be untouched: rows=%d err=%v", rows, err)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
