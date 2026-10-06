package repository

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Regression: UPDATE ... RETURNING does not preserve the claim subquery's ORDER BY,
// so two events for one aggregate in the same batch could be relayed newest-first
// (a consumer diffing snapshots then misses a price drop). Rows are inserted so the
// physical order is the reverse of created_at; the claim must still return them
// oldest-first. Runs only against a disposable Postgres (TEST_DATABASE_URL).
func TestClaimPendingKeepsCreationOrder_Postgres(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("no TEST_DATABASE_URL set; skipping Postgres outbox ordering test")
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
	ddl, err := os.ReadFile(filepath.Join("..", "..", "migrations", "0006_order_outbox.up.sql"))
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	if _, err := pool.Exec(ctx, string(ddl)); err != nil {
		t.Fatalf("apply migration: %v", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM order_outbox_events`); err != nil {
		t.Fatalf("reset table: %v", err)
	}

	base := time.Now().Add(-time.Minute)
	// Inserted newer-first so heap order disagrees with created_at.
	for _, r := range []struct {
		id  string
		age time.Duration
	}{{"evt-updated", 2 * time.Second}, {"evt-created", 0}} {
		if _, err := pool.Exec(ctx, `INSERT INTO order_outbox_events
			(event_id, aggregate_type, aggregate_id, event_type, payload, request_id, status, attempts, available_at, created_at)
			VALUES ($1, 'Agg', 'agg-1', 'T', '\x00', '', 'pending', 0, $2, $2)`, r.id, base.Add(r.age)); err != nil {
			t.Fatalf("insert %s: %v", r.id, err)
		}
	}

	got, err := NewPgOutboxRepository(pool).ClaimPending(ctx, 10, 30*time.Second)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if len(got) != 2 || got[0].EventID != "evt-created" || got[1].EventID != "evt-updated" {
		ids := make([]string, len(got))
		for i, e := range got {
			ids[i] = e.EventID
		}
		t.Fatalf("claim order = %v, want [evt-created evt-updated]", ids)
	}
}
