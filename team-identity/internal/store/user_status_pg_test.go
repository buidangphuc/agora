package store_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Postgres tests for migration 0007_user_status. Run against a
// Postgres container from testkit; migrationSQL comes from helpers_test.go.

var statusColumns = []string{"status", "status_reason", "status_until", "status_changed_at"}

func statusColumnCount(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var n int
	err := pool.QueryRow(context.Background(), `SELECT count(*) FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name='users'
		AND column_name IN ('status','status_reason','status_until','status_changed_at')`).Scan(&n)
	if err != nil {
		t.Fatalf("columns: %v", err)
	}
	return n
}

func statusIndexCount(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var n int
	err := pool.QueryRow(context.Background(), `SELECT count(*) FROM pg_indexes
		WHERE schemaname = current_schema() AND tablename='users' AND indexdef LIKE '%(status)%'`).Scan(&n)
	if err != nil {
		t.Fatalf("indexes: %v", err)
	}
	return n
}

// preStatusPool returns a database migrated to version 6 (before 0007), seeded with legacy users.
func preStatusPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool := poolAt(t, 6)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, username, password_hash) VALUES ('u-st1','u-st1','x'), ('u-st2','u-st2','x')`); err != nil {
		t.Fatalf("seed legacy users: %v", err)
	}
	return pool
}

// Scenario: User enforcement: Existing rows read as active after migration
func TestMigration0007UserStatusUpDown(t *testing.T) {
	pool := preStatusPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx, migrationSQL(t, "0007_user_status.up.sql")); err != nil {
		t.Fatalf("apply 0007 up: %v", err)
	}
	if got := statusColumnCount(t, pool); got != len(statusColumns) {
		t.Fatalf("columns after up = %d, want %d", got, len(statusColumns))
	}
	if got := statusIndexCount(t, pool); got < 1 {
		t.Fatalf("status index missing after up")
	}
	rows, err := pool.Query(ctx, `SELECT id, status, status_reason, status_until FROM users WHERE id LIKE 'u-st%' ORDER BY id`)
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	defer rows.Close()
	seen := 0
	for rows.Next() {
		var id, status string
		var reason *string
		var until any
		if err := rows.Scan(&id, &status, &reason, &until); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if status != "active" {
			t.Errorf("user %s status = %q, want active", id, status)
		}
		if reason != nil && *reason != "" {
			t.Errorf("user %s status_reason = %q, want empty", id, *reason)
		}
		if until != nil {
			t.Errorf("user %s status_until = %v, want NULL", id, until)
		}
		seen++
	}
	if seen != 2 {
		t.Fatalf("legacy users read = %d, want 2", seen)
	}

	// Up is re-runnable (IF NOT EXISTS), as 0006.
	if _, err := pool.Exec(ctx, migrationSQL(t, "0007_user_status.up.sql")); err != nil {
		t.Fatalf("0007 up not idempotent: %v", err)
	}

	if _, err := pool.Exec(ctx, migrationSQL(t, "0007_user_status.down.sql")); err != nil {
		t.Fatalf("apply 0007 down: %v", err)
	}
	if got := statusColumnCount(t, pool); got != 0 {
		t.Errorf("columns after down = %d, want 0", got)
	}
	if got := statusIndexCount(t, pool); got != 0 {
		t.Errorf("status index after down = %d, want 0", got)
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM users WHERE id LIKE 'u-st%'`).Scan(&n); err != nil || n != 2 {
		t.Errorf("users after down = %d (err %v), want 2", n, err)
	}
}

// Scenario: User enforcement: Unknown status value is refused by the schema
func TestUserStatusCheckConstraint(t *testing.T) {
	pool := preStatusPool(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, migrationSQL(t, "0007_user_status.up.sql")); err != nil {
		t.Fatalf("apply 0007 up: %v", err)
	}

	for _, ok := range []string{"active", "restricted", "suspended", "banned"} {
		if _, err := pool.Exec(ctx, `UPDATE users SET status=$1 WHERE id='u-st1'`, ok); err != nil {
			t.Errorf("status %q rejected: %v", ok, err)
		}
	}
	if _, err := pool.Exec(ctx, `UPDATE users SET status='banned' WHERE id='u-st1'`); err != nil {
		t.Fatal(err)
	}

	assertCheck := func(label string, err error) {
		t.Helper()
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "23514" {
			t.Errorf("%s: err = %v, want check_violation 23514", label, err)
		}
	}
	_, err := pool.Exec(ctx, `UPDATE users SET status='frozen' WHERE id='u-st1'`)
	assertCheck("update frozen", err)
	_, err = pool.Exec(ctx, `INSERT INTO users (id, username, password_hash, status) VALUES ('u-st3','u-st3','x','frozen')`)
	assertCheck("insert frozen", err)

	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM users WHERE id='u-st1'`).Scan(&status); err != nil || status != "banned" {
		t.Errorf("row after refused update = %q (err %v), want banned unchanged", status, err)
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM users WHERE id='u-st3'`).Scan(&n); err != nil || n != 0 {
		t.Errorf("refused insert left %d rows (err %v)", n, err)
	}
}
