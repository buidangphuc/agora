package store_test

import (
	"context"
	"io/fs"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	sdkconfig "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/migrate"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/postgres"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/testkit"

	"github.com/buidangphuc/team-order/internal/store"
	"github.com/buidangphuc/team-order/migrations"
)

// upTo is the migrations numbered 0001..last, as the deploy migrate job saw them
// before the outbox upgrade.
func upTo(t *testing.T, last string) fs.FS {
	t.Helper()
	entries, err := fs.ReadDir(migrations.Files, ".")
	if err != nil {
		t.Fatal(err)
	}
	sub := fstest.MapFS{}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".sql") && e.Name()[:4] <= last {
			b, err := fs.ReadFile(migrations.Files, e.Name())
			if err != nil {
				t.Fatal(err)
			}
			sub[e.Name()] = &fstest.MapFile{Data: b}
		}
	}
	return sub
}

// The go-sdk v1 outbox table adds topic, traceparent and seq and two indexes. A
// service upgrading from the v0 table must keep the order of the events it still
// has to publish, also for a row a v0 relayer retried (which an UPDATE moved to the
// end of the heap), and the migration must run through the deploy migrate job, whose
// files each run as one statement or one transaction (CONCURRENTLY outside one).
func TestOutboxUpgradeMigrationsKeepEventOrder(t *testing.T) {
	ctx := context.Background()
	dsn := testkit.Schema(t, testkit.SharedPostgres(t))

	// The database as the previous release left it: migrations up to 0008.
	if err := migrate.Up(ctx, dsn, upTo(t, "0008")); err != nil {
		t.Fatalf("apply the previous schema: %v", err)
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	base := time.Now().Add(-time.Hour)
	// ord-1 has three events, e1 the oldest; f1 belongs to another aggregate and
	// sits between them in time. A v0 relayer retried e1, which rewrote its row.
	for i, r := range []struct{ id, agg string }{{"e1", "ord-1"}, {"f1", "ord-2"}, {"e2", "ord-1"}, {"e3", "ord-1"}} {
		at := base.Add(time.Duration(i) * time.Minute)
		if _, err := pool.Exec(ctx, `INSERT INTO order_outbox_events
			(event_id, aggregate_type, aggregate_id, event_type, payload, request_id, status, attempts, available_at, created_at)
			VALUES ($1, 'Order', $2, 'T', $3, '', 'pending', 0, $4, $4)`, r.id, r.agg, []byte(r.id), at); err != nil {
			t.Fatalf("seed %s: %v", r.id, err)
		}
	}
	if _, err := pool.Exec(ctx, `UPDATE order_outbox_events SET attempts = 2, error = 'broker down', available_at = now() WHERE event_id = 'e1'`); err != nil {
		t.Fatal(err)
	}

	// The upgrade, as the deploy job applies it.
	if err := migrate.Up(ctx, dsn, migrations.Files); err != nil {
		t.Fatalf("outbox upgrade: %v", err)
	}

	rows, err := pool.Query(ctx, `SELECT event_id FROM order_outbox_events ORDER BY seq`)
	if err != nil {
		t.Fatalf("seq column missing after the upgrade: %v", err)
	}
	order, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"e1", "f1", "e2", "e3"}; !slices.Equal(order, want) {
		t.Fatalf("order by seq = %v, want %v (created_at order, the retried row first)", order, want)
	}
	var distinct, nulls int
	var notNull bool
	if err := pool.QueryRow(ctx, `SELECT count(DISTINCT seq), count(*) FILTER (WHERE seq IS NULL),
		(SELECT is_nullable = 'NO' FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'order_outbox_events' AND column_name = 'seq')
		FROM order_outbox_events`).Scan(&distinct, &nulls, &notNull); err != nil {
		t.Fatal(err)
	}
	if distinct != 4 || nulls != 0 || !notNull {
		t.Fatalf("seq distinct=%d null=%d not null=%v, want 4 unique numbers and a NOT NULL column", distinct, nulls, notNull)
	}

	// Indexes: the three v1 ones exist and are valid (a failed CONCURRENTLY build
	// leaves an invalid index), the v0 claim index is gone, and no helper
	// constraint is left behind.
	for _, name := range []string{"order_outbox_events_status_idx", "order_outbox_events_agg_idx", "order_outbox_events_pending_idx"} {
		var valid bool
		if err := pool.QueryRow(ctx, `SELECT i.indisvalid FROM pg_index i JOIN pg_class c ON c.oid = i.indexrelid
			WHERE c.relname = $1 AND c.relnamespace = current_schema()::regnamespace`, name).Scan(&valid); err != nil || !valid {
			t.Errorf("index %s valid = %v (err %v)", name, valid, err)
		}
	}
	var left int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM pg_class WHERE relname = 'idx_order_outbox_claim' AND relnamespace = current_schema()::regnamespace`).Scan(&left); err != nil || left != 0 {
		t.Errorf("v0 claim index left = %d (err %v)", left, err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM pg_constraint WHERE conname = 'order_outbox_events_seq_nn'`).Scan(&left); err != nil || left != 0 {
		t.Errorf("seq_nn helper constraint left = %d (err %v)", left, err)
	}

	// An event enqueued by the upgraded service takes a later number and the SDK
	// relayer publishes ord-1 in creation order, the retried row first.
	queue, err := outbox.NewStore(pool, store.OutboxTable)
	if err != nil {
		t.Fatal(err)
	}
	if err := postgres.WithTx(ctx, pool, func(tx pgx.Tx) error {
		return queue.EnqueueTx(ctx, tx, store.OutboxRow{EventID: "e4", AggregateType: "Order", AggregateID: "ord-1", EventType: "T", Payload: []byte("e4")})
	}); err != nil {
		t.Fatal(err)
	}
	pub := testkit.NewRecorder()
	relayer, err := outbox.NewRelayer(queue, pub, outbox.RelayerOptions{Topic: "order.events", Config: sdkconfig.Outbox{BatchSize: 10, ClaimLock: time.Minute}})
	if err != nil {
		t.Fatal(err)
	}
	for range 5 {
		if _, err := relayer.RelayOnce(ctx); err != nil {
			t.Fatal(err)
		}
	}
	var ord1 []string
	for _, r := range pub.Records() {
		if string(r.Key) == "ord-1" {
			ord1 = append(ord1, string(r.Value))
		}
	}
	if want := []string{"e1", "e2", "e3", "e4"}; !slices.Equal(ord1, want) {
		t.Fatalf("relayed events of ord-1 = %v, want %v", ord1, want)
	}
}
