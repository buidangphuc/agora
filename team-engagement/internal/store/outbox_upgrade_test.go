package store

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	sdkconfig "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/kafkax"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/testkit"
)

// firstV1Migration is the first migration of the go-sdk v1 outbox upgrade.
const firstV1Migration = "0012"

func queryStrings(t *testing.T, pool *pgxpool.Pool, q string) []string {
	t.Helper()
	rows, err := pool.Query(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// A table in the go-sdk v0 shape (no seq, no topic, no traceparent) that holds
// rows is upgraded by the migrations: every row gets a seq in (created_at,
// event_id) order, a retried row (updated after its insert, so it sits behind
// newer rows in the heap) keeps its creation slot, the v1 indexes exist and the
// v0 claim index is gone. The SDK relayer then publishes one aggregate in
// creation order.
func TestOutboxUpgradeKeepsCreationOrder(t *testing.T) {
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, pgURL(t))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if _, err := pool.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
		t.Fatal(err)
	}
	applyMigrations(t, pool, "", firstV1Migration)

	// Aggregate Listing/A: five events; a1 is the oldest and is retried (updated
	// after a2..a5 were inserted). a4 and a5 share a created_at, as facts of one
	// transaction do. Listing/B has one pending and one published row.
	base := time.Date(2026, 10, 9, 8, 0, 0, 0, time.UTC)
	for _, r := range []struct {
		id, agg, status string
		at              time.Duration
	}{
		{"a2", "A", "pending", time.Second},
		{"a1", "A", "pending", 0},
		{"a3", "A", "pending", 2 * time.Second},
		{"a5", "A", "pending", 3 * time.Second},
		{"a4", "A", "pending", 3 * time.Second},
		{"b1", "B", "pending", 500 * time.Millisecond},
		{"p1", "B", "published", -time.Hour},
	} {
		if _, err := pool.Exec(ctx, `INSERT INTO `+OutboxTable+`
			(event_id, aggregate_type, aggregate_id, event_type, payload, status, published_at, created_at)
			VALUES ($1, 'Listing', $2, 'platform.engagement.v1.FavoriteAdded', $3, $4,
			        CASE WHEN $4 = 'published' THEN now() END, $5)`,
			r.id, r.agg, []byte("payload-"+r.id), r.status, base.Add(r.at)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, `UPDATE `+OutboxTable+`
		SET attempts = 3, available_at = now() - interval '1 second', error = 'broker down' WHERE event_id = 'a1'`); err != nil {
		t.Fatal(err)
	}

	applyMigrations(t, pool, firstV1Migration, "")

	wantOrder := []string{"p1", "a1", "b1", "a2", "a3", "a4", "a5"} // created_at, event_id
	if got := queryStrings(t, pool, `SELECT event_id FROM `+OutboxTable+` ORDER BY seq`); !reflect.DeepEqual(got, wantOrder) {
		t.Fatalf("rows by seq = %v, want creation order %v", got, wantOrder)
	}
	var unnumbered int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM `+OutboxTable+` WHERE seq IS NULL`).Scan(&unnumbered); err != nil || unnumbered != 0 {
		t.Fatalf("%d rows without seq (%v)", unnumbered, err)
	}
	var notNull bool
	if err := pool.QueryRow(ctx, `SELECT attnotnull FROM pg_attribute
		WHERE attrelid = $1::regclass AND attname = 'seq'`, OutboxTable).Scan(&notNull); err != nil || !notNull {
		t.Fatalf("seq NOT NULL = %v (%v)", notNull, err)
	}
	// The v1 indexes are valid and the v0 claim index is gone.
	idx := queryStrings(t, pool, `SELECT c.relname FROM pg_index i JOIN pg_class c ON c.oid = i.indexrelid
		WHERE i.indrelid = $$`+OutboxTable+`$$::regclass AND i.indisvalid ORDER BY c.relname`)
	wantIdx := []string{OutboxTable + "_agg_idx", OutboxTable + "_pending_idx", OutboxTable + "_pkey", OutboxTable + "_status_idx"}
	if !reflect.DeepEqual(idx, wantIdx) {
		t.Fatalf("indexes = %v, want %v", idx, wantIdx)
	}

	// Rows written after the upgrade queue behind every existing one.
	if _, err := pool.Exec(ctx, `INSERT INTO `+OutboxTable+`
		(event_id, aggregate_type, aggregate_id, event_type, payload) VALUES ('a6', 'Listing', 'A', 't', '\x00')`); err != nil {
		t.Fatal(err)
	}

	pub := testkit.NewRecorder()
	relayer, err := outbox.NewRelayer(testOutbox(t, pool), pub, outbox.RelayerOptions{
		Topic:  "engagement.events",
		Config: sdkconfig.Outbox{PollInterval: time.Second, BatchSize: 20, ClaimLock: time.Minute},
	})
	if err != nil {
		t.Fatal(err)
	}
	rep, err := relayer.RelayOnce(ctx)
	if err != nil || rep.Claimed != 7 || rep.Published != 7 {
		t.Fatalf("RelayOnce = %+v, %v, want the 7 unpublished rows relayed and the published one left alone", rep, err)
	}
	var a []string
	for _, rec := range pub.Records() {
		if string(rec.Key) != "A" {
			continue
		}
		for _, h := range rec.Headers {
			if h.Key == kafkax.HeaderEventID {
				a = append(a, string(h.Value))
			}
		}
	}
	if want := []string{"a1", "a2", "a3", "a4", "a5", "a6"}; !reflect.DeepEqual(a, want) {
		t.Fatalf("aggregate A published in order %v, want creation order %v", a, want)
	}
}
