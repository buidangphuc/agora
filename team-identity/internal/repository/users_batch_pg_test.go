package repository

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// GetByIDs returns only the known users (id + username), once each. Runs only
// against a disposable Postgres (TEST_DATABASE_URL).
func TestGetByIDs_Postgres(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("no TEST_DATABASE_URL set; skipping Postgres GetByIDs test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Skipf("cannot connect to Postgres (%v); skipping", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		t.Skipf("Postgres unreachable (%v); skipping", err)
	}
	ddl, err := os.ReadFile(filepath.Join("..", "..", "migrations", "0001_users.up.sql"))
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	if _, err := pool.Exec(ctx, string(ddl)); err != nil {
		t.Fatalf("apply migration: %v", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM users WHERE id IN ('u-batch-1','u-batch-2')`); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, username, password_hash) VALUES
		('u-batch-1','batch-one','x'), ('u-batch-2','batch-two','x')`); err != nil {
		t.Fatalf("seed: %v", err)
	}
	got, err := NewPostgresUserRepository(pool).GetByIDs(ctx, []string{"u-batch-1", "u-batch-2", "nope"})
	if err != nil {
		t.Fatalf("GetByIDs: %v", err)
	}
	names := map[string]string{}
	for _, u := range got {
		names[u.ID] = u.Username
		if u.PasswordHash != "" {
			t.Fatalf("password hash must not be loaded")
		}
	}
	if len(names) != 2 || names["u-batch-1"] != "batch-one" || names["u-batch-2"] != "batch-two" {
		t.Fatalf("got %v", names)
	}
}
