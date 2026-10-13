package store

import (
	"context"
	"testing"
)

// GetByIDs returns only the known users (id + username), once each. Runs only
// against a disposable Postgres (testkit).
func TestGetByIDs_Postgres(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
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
