package events_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/testkit"

	"github.com/buidangphuc/team-engagement/internal/events"
)

const table = "engagement_outbox_events"

func retentionPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, testkit.Postgres(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(ctx, outbox.SchemaSQL(table)); err != nil {
		t.Fatal(err)
	}
	for _, r := range []struct{ id, status, ago string }{
		{"old-published", "published", "8 days"},
		{"fresh-published", "published", "1 day"},
		{"old-pending", "pending", "30 days"},
		{"old-parked", "failed", "30 days"},
	} {
		if _, err := pool.Exec(ctx, `INSERT INTO `+table+`
			(event_id, aggregate_type, aggregate_id, event_type, payload, status, published_at, created_at)
			VALUES ($1, 'Listing', 'L', 't', '\x00', $2, CASE WHEN $2 = 'published' THEN now() - $3::interval END, now() - $3::interval)`,
			r.id, r.status, r.ago); err != nil {
			t.Fatal(err)
		}
	}
	return pool
}

func remaining(t *testing.T, pool *pgxpool.Pool) map[string]bool {
	t.Helper()
	rows, err := pool.Query(context.Background(), `SELECT event_id FROM `+table)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		out[id] = true
	}
	return out
}

// Published rows older than the retention window are deleted; fresh published,
// pending and parked rows stay (a parked row is evidence, a pending one is work).
func TestRetentionDeletesOnlyOldPublishedRows(t *testing.T) {
	pool := retentionPool(t)
	logger := slog.New(slog.DiscardHandler)
	r := events.NewRetention(pool, table, 7*24*time.Hour, time.Hour, logger)

	n, err := r.Sweep(context.Background())
	if err != nil || n != 1 {
		t.Fatalf("Sweep = %d, %v, want 1 row deleted", n, err)
	}
	got := remaining(t, pool)
	if got["old-published"] || !got["fresh-published"] || !got["old-pending"] || !got["old-parked"] || len(got) != 3 {
		t.Fatalf("remaining = %v", got)
	}
}

// As a lifecycle component it sweeps at start and stops cleanly when cancelled.
func TestRetentionRunSweepsAtStartAndStops(t *testing.T) {
	pool := retentionPool(t)
	logger := slog.New(slog.DiscardHandler)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- events.NewRetention(pool, table, 7*24*time.Hour, time.Hour, logger).Run(ctx) }()

	deadline := time.Now().Add(10 * time.Second)
	for remaining(t, pool)["old-published"] {
		if time.Now().After(deadline) {
			t.Fatal("old published row was not deleted at start")
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run = %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not stop")
	}
}
