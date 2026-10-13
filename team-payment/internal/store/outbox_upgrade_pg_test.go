package store_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/migrate"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/testkit"

	"github.com/buidangphuc/team-payment/internal/pgtest"
	"github.com/buidangphuc/team-payment/migrations"
)

// A payment_db at 0007 holds a v0 outbox table (0003) with pending, retried and published
// rows. Applying 0008..0017 through the migrate runner (the same files the deploy job runs,
// including the CONCURRENTLY index files) must number every row in creation order, leave the
// v1 schema, and let the SDK relayer publish each aggregate in creation order.
func TestOutboxUpgradeFromV0_Postgres(t *testing.T) {
	ctx := context.Background()
	dsn := pgtest.SchemaURL(t)
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	// The previous schema: every migration before 0008, then the runner's bookkeeping at 0007.
	for _, f := range pgtest.UpFiles(t) {
		if filepath.Base(f) < "0008_" {
			pgtest.Apply(t, pool, f)
		}
	}
	mustExec(t, pool, `CREATE TABLE schema_migrations (version bigint NOT NULL PRIMARY KEY, dirty boolean NOT NULL)`)
	mustExec(t, pool, `INSERT INTO schema_migrations VALUES (7, false)`)

	t0 := time.Now().Add(-time.Hour).Truncate(time.Second)
	insert := func(id, agg, status string, offset time.Duration) {
		t.Helper()
		mustExec(t, pool, `INSERT INTO payment_outbox_events
			(event_id, aggregate_type, aggregate_id, event_type, payload, request_id, status, attempts, available_at, created_at)
			VALUES ($1, 'Payment', $2, 'T', convert_to($1, 'UTF8'), '', $3, 0, $4, $4)`, id, agg, status, t0.Add(offset))
	}
	// Heap order is insertion order; creation order is the offset.
	insert("e1", "order-1", "pending", 0)
	insert("e2", "order-1", "pending", 2*time.Second)
	insert("e3", "order-1", "pending", 4*time.Second)
	insert("f1", "order-2", "pending", time.Second)
	insert("f2", "order-2", "pending", 3*time.Second)
	insert("p1", "order-3", "published", 5*time.Second)
	// A retried row: the update rewrites the tuple, so it now sits after e2 and e3 in the heap
	// (a BIGSERIAL seq would number it last).
	mustExec(t, pool, `UPDATE payment_outbox_events SET attempts = 1, available_at = now() WHERE event_id = 'e1'`)

	if err := migrate.Up(ctx, dsn, migrations.Files); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	pool.Reset()

	// seq follows creation order across every row.
	rows, err := pool.Query(ctx, `SELECT event_id FROM payment_outbox_events ORDER BY seq`)
	if err != nil {
		t.Fatal(err)
	}
	var seqOrder []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		seqOrder = append(seqOrder, id)
	}
	rows.Close()
	if got, want := strings.Join(seqOrder, ","), "e1,f1,e2,f2,e3,p1"; got != want {
		t.Fatalf("rows by seq = %s, want %s", got, want)
	}

	// The v1 schema: seq NOT NULL with a default, the new indexes valid, claim_idx gone, and no
	// leftover check constraint.
	if scalar[string](t, pool, `SELECT is_nullable FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = 'payment_outbox_events' AND column_name = 'seq'`) != "NO" {
		t.Error("seq is still nullable")
	}
	if scalar[string](t, pool, `SELECT column_default FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = 'payment_outbox_events' AND column_name = 'seq'`) == "" {
		t.Error("seq has no default")
	}
	for _, idx := range []string{"payment_outbox_events_status_idx", "payment_outbox_events_agg_idx", "payment_outbox_events_pending_idx"} {
		if scalar[int](t, pool, `SELECT count(*) FROM pg_index i JOIN pg_class c ON c.oid = i.indexrelid
			WHERE c.relname = $1 AND c.relnamespace = current_schema()::regnamespace AND i.indisvalid`, idx) != 1 {
			t.Errorf("index %s is missing or invalid", idx)
		}
	}
	if n := scalar[int](t, pool, `SELECT count(*) FROM pg_class WHERE relname = 'payment_outbox_events_claim_idx' AND relnamespace = current_schema()::regnamespace`); n != 0 {
		t.Error("claim_idx still exists")
	}
	if n := scalar[int](t, pool, `SELECT count(*) FROM pg_constraint WHERE conname = 'payment_outbox_events_seq_nn'`); n != 0 {
		t.Error("the temporary seq check constraint was left behind")
	}

	// A row enqueued by the SDK after the upgrade gets the next number.
	st, err := outbox.NewStore(pool, "payment_outbox_events")
	if err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.EnqueueTx(ctx, tx, outbox.Row{EventID: "g1", AggregateType: "Payment", AggregateID: "order-1", EventType: "T", Payload: []byte("g1")}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// The SDK relayer publishes each aggregate in creation order, the retried row first.
	rec := testkit.NewRecorder()
	rl, err := outbox.NewRelayer(st, rec, outbox.RelayerOptions{Topic: "payment.events"})
	if err != nil {
		t.Fatal(err)
	}
	for range 5 {
		if _, err := rl.RelayOnce(ctx); err != nil {
			t.Fatalf("relay: %v", err)
		}
	}
	byAgg := map[string][]string{}
	for _, r := range rec.Records() {
		byAgg[string(r.Key)] = append(byAgg[string(r.Key)], string(r.Value))
	}
	if got := strings.Join(byAgg["order-1"], ","); got != "e1,e2,e3,g1" {
		t.Errorf("order-1 published %s, want e1,e2,e3,g1", got)
	}
	if got := strings.Join(byAgg["order-2"], ","); got != "f1,f2" {
		t.Errorf("order-2 published %s, want f1,f2", got)
	}
	if _, ok := byAgg["order-3"]; ok {
		t.Errorf("the already published row was published again: %v", byAgg["order-3"])
	}
}
