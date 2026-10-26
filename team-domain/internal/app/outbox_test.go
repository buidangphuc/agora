package app_test

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"
	"github.com/buidangphuc/team-domain/internal/app"
)

func insertOldRow(t *testing.T, pool *pgxpool.Pool, id, aggregate, status string, attempts int, createdAt time.Time) {
	t.Helper()
	if _, err := pool.Exec(t.Context(), `INSERT INTO outbox_events
		(event_id, aggregate_type, aggregate_id, event_type, payload, request_id, status, attempts, error, available_at, created_at)
		VALUES ($1, 'Listing', $2, 'platform.listing.v1.ListingChanged', $3, '', $4::text, $5, CASE WHEN $4::text = 'failed' THEN 'max attempts' END, $6, $6)`,
		id, aggregate, []byte(id), status, attempts, createdAt); err != nil {
		t.Fatalf("insert %s: %v", id, err)
	}
}

func valuesByKey(recs []string, keys []string) map[string][]string {
	out := map[string][]string{}
	for i, v := range recs {
		out[keys[i]] = append(out[keys[i]], v)
	}
	return out
}

func TestUpgradedPendingRowsArePublished(t *testing.T) {
	// scenario: Domain SDK: Upgraded pending rows are relayed
	url := migratedUpTo(t, 12)
	pool := openPool(t, url)
	base := time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)
	for i, r := range []struct{ id, agg string }{
		{"e1", "L1"}, {"e2", "L2"}, {"e3", "L1"}, {"e4", "L2"}, {"e5", "L1"},
	} {
		insertOldRow(t, pool, r.id, r.agg, "pending", 0, base.Add(time.Duration(i)*time.Minute))
	}
	migrateEnv(t, url)
	if err := app.Migrate(t.Context()); err != nil {
		t.Fatalf("migrate up: %v", err)
	}

	brokers := testkit.Redpanda(t)
	r := start(t, domainSettings(testkit.FreePort(t), url, brokers))
	waitServing(t, r)

	recs := testkit.ReadAll(t, brokers, listingTopic, 5, 60*time.Second)
	values, keys := make([]string, len(recs)), make([]string, len(recs))
	for i, rec := range recs {
		values[i], keys[i] = string(rec.Value), string(rec.Key)
	}
	got := valuesByKey(values, keys)
	want := map[string][]string{"L1": {"e1", "e3", "e5"}, "L2": {"e2", "e4"}}
	for agg, ids := range want {
		if len(got[agg]) != len(ids) {
			t.Fatalf("records for %s = %v, want %v", agg, got[agg], ids)
		}
		for i := range ids {
			if got[agg][i] != ids[i] {
				t.Fatalf("records for %s = %v, want seq order %v", agg, got[agg], ids)
			}
		}
	}
	testkit.Eventually(t, 20*time.Second, func() bool {
		var n int
		err := pool.QueryRow(t.Context(), `SELECT count(*) FROM outbox_events WHERE status = 'published'`).Scan(&n)
		return err == nil && n == 5
	})
}

func TestUpgradedFailedRowsAreRequeuedAndPublished(t *testing.T) {
	// scenario: Domain SDK: Upgraded failed rows are requeued and published
	url := migratedUpTo(t, 12)
	pool := openPool(t, url)
	base := time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)
	insertOldRow(t, pool, "f-a", "LA", "failed", 5, base)
	insertOldRow(t, pool, "p-a", "LA", "pending", 0, base.Add(time.Minute))
	insertOldRow(t, pool, "p-b", "LB", "pending", 0, base.Add(2*time.Minute))
	migrateEnv(t, url)
	if err := app.Migrate(t.Context()); err != nil {
		t.Fatalf("migrate up: %v", err)
	}

	for _, id := range []string{"f-a", "p-a", "p-b"} {
		if st, attempts := outboxStatus(t, pool, id); st != "pending" || attempts != 0 {
			t.Fatalf("%s after migrate up = %s attempts %d, want pending 0", id, st, attempts)
		}
	}

	brokers := testkit.Redpanda(t)
	r := start(t, domainSettings(testkit.FreePort(t), url, brokers))
	waitServing(t, r)

	testkit.Eventually(t, 15*time.Second, func() bool {
		var n int
		err := pool.QueryRow(t.Context(), `SELECT count(*) FROM outbox_events WHERE status = 'published'`).Scan(&n)
		return err == nil && n == 3
	})
	recs := testkit.ReadAll(t, brokers, listingTopic, 3, 30*time.Second)
	var order []string
	for _, rec := range recs {
		if string(rec.Key) == "LA" {
			order = append(order, string(rec.Value))
		}
	}
	if len(order) != 2 || order[0] != "f-a" || order[1] != "p-a" {
		t.Fatalf("records for LA = %v, want [f-a p-a] in seq order", order)
	}
}

func TestRunPurgesPublishedOutboxRows(t *testing.T) {
	// scenario: Outbox purge: The service runs the purge on its schedule
	app.SetPurgeInterval(t, 200*time.Millisecond)
	url := migratedURL(t)
	pool := openPool(t, url)
	now := time.Now()
	ins := func(id, status string, age time.Duration) {
		t.Helper()
		at := now.Add(-age)
		if _, err := pool.Exec(t.Context(), `INSERT INTO outbox_events
			(event_id, aggregate_type, aggregate_id, event_type, payload, request_id, status, attempts, available_at, created_at, published_at)
			VALUES ($1, 'Listing', $1, 'platform.listing.v1.ListingChanged', $2, '', $3::text, 1, $4::timestamptz, $4::timestamptz, CASE WHEN $3::text = 'published' THEN $4::timestamptz END)`,
			id, []byte(id), status, at); err != nil {
			t.Fatalf("insert %s: %v", id, err)
		}
	}
	ins("pub-old", "published", 8*24*time.Hour)
	ins("pub-recent", "published", 6*24*time.Hour)
	ins("pending-old", "pending", 30*24*time.Hour)
	ins("failed-old", "failed", 30*24*time.Hour)

	r := start(t, domainSettings(testkit.FreePort(t), url, deadBroker))
	waitServing(t, r)

	count := func(id string) int {
		var n int
		if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM outbox_events WHERE event_id = $1`, id).Scan(&n); err != nil {
			t.Fatalf("count %s: %v", id, err)
		}
		return n
	}
	testkit.Eventually(t, 5*time.Second, func() bool { return count("pub-old") == 0 })
	for _, id := range []string{"pub-recent", "pending-old", "failed-old"} {
		if count(id) != 1 {
			t.Errorf("row %s was purged; only published rows older than 7 days may be", id)
		}
	}
}
