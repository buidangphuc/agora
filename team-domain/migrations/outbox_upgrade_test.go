package migrations_test

import (
	"context"
	"fmt"
	"io/fs"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/migrate"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/postgres"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/testkit"

	"github.com/buidangphuc/team-domain/migrations"
)

// upTo returns the migrations whose version is at most n, so a test can stop at
// the schema the previous release ran on.
func upTo(t *testing.T, n int) fs.FS {
	t.Helper()
	out := fstest.MapFS{}
	entries, err := fs.ReadDir(migrations.Files, ".")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		ver, _, _ := strings.Cut(e.Name(), "_")
		v, err := strconv.Atoi(ver)
		if err != nil || v > n {
			continue
		}
		b, err := fs.ReadFile(migrations.Files, e.Name())
		if err != nil {
			t.Fatal(err)
		}
		out[e.Name()] = &fstest.MapFile{Data: b}
	}
	return out
}

// TestOutboxUpgradeKeepsCreationOrderAndInFlightRows applies migrations up to the
// last one the v0 outbox ran on (0010), fills outbox_events the way the v0 writer
// and relayer did (a retried row whose tuple was rewritten behind newer events of
// its aggregate, two events of one transaction tying on created_at, an in-flight
// leased row, a published row), applies migrations 0011 onwards, and checks the
// table has the go-sdk v1 shape, seq follows (created_at, event_id), in-flight
// rows are untouched and the SDK relayer publishes each aggregate in creation order.
func TestOutboxUpgradeKeepsCreationOrderAndInFlightRows(t *testing.T) {
	ctx := context.Background()
	dsn := testkit.Schema(t, testkit.SharedPostgres(t))
	if err := migrate.Up(ctx, dsn, upTo(t, 10)); err != nil {
		t.Fatalf("apply migrations up to 0010: %v", err)
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	base := time.Now().Add(-time.Hour).Truncate(time.Second)
	insert := func(id, agg string, age time.Duration) {
		t.Helper()
		if _, err := pool.Exec(ctx, `INSERT INTO outbox_events
			(event_id, aggregate_type, aggregate_id, event_type, payload, request_id, status, attempts, available_at, created_at)
			VALUES ($1, 'Listing', $2, 'T', $3, 'req-'||$1, 'pending', 0, $4, $4)`,
			id, agg, []byte("payload-"+id), base.Add(age)); err != nil {
			t.Fatalf("insert %s: %v", id, err)
		}
	}
	// Heap order is the order of these inserts, which disagrees with creation order.
	insert("e-2a", "L1", 2*time.Second) // ties with e-2b on created_at: event_id decides
	insert("e-2b", "L1", 2*time.Second)
	insert("e-3", "L1", 3*time.Second)
	insert("e-1", "L1", time.Second)
	insert("m-1", "L2", time.Second)
	insert("m-2", "L2", 2*time.Second)
	insert("old", "L1", 0)
	// e-1 was retried by the v0 relayer: its tuple is rewritten, so it now sits
	// behind e-2a..e-3 in the heap. attempts and error must survive the upgrade.
	if _, err := pool.Exec(ctx, `UPDATE outbox_events
		SET attempts = 2, error = 'broker down', available_at = now() - interval '1 second' WHERE event_id = 'e-1'`); err != nil {
		t.Fatal(err)
	}
	// m-2 is leased by a v0 relayer whose lease has since expired.
	if _, err := pool.Exec(ctx, `UPDATE outbox_events SET locked_until = now() - interval '1 minute' WHERE event_id = 'm-2'`); err != nil {
		t.Fatal(err)
	}
	// old was published long ago and must not be sent again.
	if _, err := pool.Exec(ctx, `UPDATE outbox_events SET status = 'published', published_at = now() WHERE event_id = 'old'`); err != nil {
		t.Fatal(err)
	}

	if err := migrate.Up(ctx, dsn, migrations.Files); err != nil {
		t.Fatalf("apply migrations 0011 onwards: %v", err)
	}

	// seq follows (created_at, event_id) over all rows, gap-free from 1.
	rows, err := pool.Query(ctx, `SELECT event_id FROM outbox_events ORDER BY seq`)
	if err != nil {
		t.Fatal(err)
	}
	var bySeq []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		bySeq = append(bySeq, id)
	}
	rows.Close()
	want := []string{"old", "e-1", "m-1", "e-2a", "e-2b", "m-2", "e-3"}
	if fmt.Sprint(bySeq) != fmt.Sprint(want) {
		t.Fatalf("event ids by seq = %v, want %v", bySeq, want)
	}

	// In-flight state is untouched.
	var attempts int
	var errText string
	var status string
	if err := pool.QueryRow(ctx, `SELECT attempts, error, status FROM outbox_events WHERE event_id = 'e-1'`).Scan(&attempts, &errText, &status); err != nil {
		t.Fatal(err)
	}
	if attempts != 2 || errText != "broker down" || status != "pending" {
		t.Fatalf("e-1 = attempts %d error %q status %q, want 2 / broker down / pending", attempts, errText, status)
	}

	// The table now has the v1 shape: seq NOT NULL with a default, the three v1
	// indexes, and no claim_idx.
	var notNull bool
	var def *string
	if err := pool.QueryRow(ctx, `SELECT a.attnotnull, pg_get_expr(d.adbin, d.adrelid)
		FROM pg_attribute a LEFT JOIN pg_attrdef d ON d.adrelid = a.attrelid AND d.adnum = a.attnum
		WHERE a.attrelid = 'outbox_events'::regclass AND a.attname = 'seq'`).Scan(&notNull, &def); err != nil {
		t.Fatal(err)
	}
	if !notNull || def == nil || !strings.Contains(*def, "outbox_events_seq_seq") {
		t.Fatalf("seq notnull=%v default=%v, want NOT NULL with the sequence default", notNull, def)
	}
	idx := map[string]bool{}
	irows, err := pool.Query(ctx, `SELECT indexname FROM pg_indexes WHERE tablename = 'outbox_events' AND schemaname = current_schema()`)
	if err != nil {
		t.Fatal(err)
	}
	for irows.Next() {
		var name string
		if err := irows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		idx[name] = true
	}
	irows.Close()
	for _, name := range []string{"outbox_events_status_idx", "outbox_events_agg_idx", "outbox_events_pending_idx"} {
		if !idx[name] {
			t.Errorf("index %s missing after the upgrade: have %v", name, idx)
		}
	}
	if idx["outbox_events_claim_idx"] {
		t.Error("outbox_events_claim_idx still exists after the upgrade")
	}

	// The v1 store enqueues into the upgraded table, and its row sorts after all old ones.
	store, err := outbox.NewStore(pool, "outbox_events")
	if err != nil {
		t.Fatal(err)
	}
	if err := postgres.WithTx(ctx, pool, func(tx pgx.Tx) error {
		return store.EnqueueTx(ctx, tx, outbox.Row{EventID: "e-4", AggregateType: "Listing", AggregateID: "L1", EventType: "T", Payload: []byte("payload-e-4")})
	}); err != nil {
		t.Fatalf("enqueue on the upgraded table: %v", err)
	}

	// The SDK relayer publishes each aggregate in creation order, skips the
	// published row, and keys each record by the aggregate id.
	rec := testkit.NewRecorder()
	relayer, err := outbox.NewRelayer(store, rec, outbox.RelayerOptions{
		Topic:  "listing.events",
		Config: config.Outbox{PollInterval: 10 * time.Millisecond, BatchSize: 100, ClaimLock: time.Minute},
	})
	if err != nil {
		t.Fatal(err)
	}
	for range 10 {
		rep, err := relayer.RelayOnce(ctx)
		if err != nil {
			t.Fatalf("RelayOnce: %v", err)
		}
		if rep.Published == 0 {
			break
		}
	}
	got := map[string][]string{}
	for _, r := range rec.Records() {
		if r.Topic != "listing.events" {
			t.Errorf("record topic = %q, want listing.events", r.Topic)
		}
		got[string(r.Key)] = append(got[string(r.Key)], strings.TrimPrefix(string(r.Value), "payload-"))
	}
	wantByAgg := map[string]string{
		"L1": fmt.Sprint([]string{"e-1", "e-2a", "e-2b", "e-3", "e-4"}),
		"L2": fmt.Sprint([]string{"m-1", "m-2"}),
	}
	for agg, w := range wantByAgg {
		if fmt.Sprint(got[agg]) != w {
			t.Errorf("aggregate %s published %v, want %s", agg, got[agg], w)
		}
	}
	if len(got) != len(wantByAgg) {
		t.Errorf("published aggregates %v, want only L1 and L2 (the published row is not sent again)", got)
	}
}
