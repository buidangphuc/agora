package store

import (
	"context"
	"io/fs"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5"

	sdkconfig "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/migrate"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/outbox"
	sdkpgx "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/postgres"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/testkit"

	"github.com/buidangphuc/team-chat/migrations"
)

// upTo returns the migrations whose number is at most n, the schema a v0 deploy
// has.
func upTo(t *testing.T, n int) fs.FS {
	t.Helper()
	out := fstest.MapFS{}
	entries, err := fs.ReadDir(migrations.Files, ".")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		var num int
		for _, c := range e.Name()[:4] {
			num = num*10 + int(c-'0')
		}
		if num > n {
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

// A v0 outbox table (migration 0003, no seq) that holds rows is upgraded by the
// forward migrations without losing order: seq follows creation order
// (created_at, then event_id), also for a retried row that an update moved behind
// newer rows of its aggregate in the heap, and the SDK relayer then publishes an
// aggregate's rows in creation order. Runs against a testkit Postgres container.
func TestOutboxV1UpgradeKeepsCreationOrder_Postgres(t *testing.T) {
	ctx := context.Background()
	url := testkit.Postgres(t)
	if err := migrate.Up(ctx, url, upTo(t, 3)); err != nil {
		t.Fatalf("migrate to v0 schema: %v", err)
	}
	pool, err := sdkpgx.Open(ctx, sdkconfig.Postgres{URL: url, MaxConns: 4})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	t0 := time.Now().Add(-time.Hour).Truncate(time.Microsecond)
	// Heap order is deliberately not creation order. thread-A: a2 is created
	// before a3 and a1 ties with a2 on created_at (one transaction); a1 was
	// retried, so the update rewrote it behind the newer rows.
	seed := []struct {
		id, agg string
		age     time.Duration
		status  string
	}{
		{"a3", "thread-A", 3 * time.Second, "pending"},
		{"a2", "thread-A", 1 * time.Second, "pending"},
		{"a1", "thread-A", 1 * time.Second, "pending"},
		{"b1", "thread-B", 2 * time.Second, "pending"},
		{"done", "thread-A", 0, "published"},
	}
	for _, r := range seed {
		if _, err := pool.Exec(ctx, `INSERT INTO chat_outbox_events
			(event_id, aggregate_type, aggregate_id, event_type, payload, status, available_at, created_at)
			VALUES ($1, 'ChatThread', $2, 'T', convert_to($1, 'UTF8'), $3, $4, $4)`,
			r.id, r.agg, r.status, t0.Add(r.age)); err != nil {
			t.Fatalf("seed %s: %v", r.id, err)
		}
	}
	if _, err := pool.Exec(ctx, `UPDATE chat_outbox_events SET attempts = 1, error = 'broker unavailable', available_at = now() WHERE event_id = 'a1'`); err != nil {
		t.Fatal(err)
	}

	if err := migrate.Up(ctx, url, migrations.Files); err != nil {
		t.Fatalf("migrate to v1 schema: %v", err)
	}

	// seq is set on every row, unique, and orders rows by (created_at, event_id).
	rows, err := pool.Query(ctx, `SELECT event_id FROM chat_outbox_events ORDER BY seq`)
	if err != nil {
		t.Fatalf("read seq: %v", err)
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
	if want := []string{"done", "a1", "a2", "b1", "a3"}; !slices.Equal(bySeq, want) {
		t.Fatalf("order by seq = %v, want %v", bySeq, want)
	}
	var distinct, nulls int
	if err := pool.QueryRow(ctx, `SELECT count(DISTINCT seq), count(*) FILTER (WHERE seq IS NULL) FROM chat_outbox_events`).Scan(&distinct, &nulls); err != nil || distinct != len(seed) || nulls != 0 {
		t.Fatalf("distinct seq = %d, null seq = %d, err = %v; want %d and 0", distinct, nulls, err, len(seed))
	}

	// seq is NOT NULL and new rows are numbered by the default, after the old ones.
	if _, err := pool.Exec(ctx, `UPDATE chat_outbox_events SET seq = NULL WHERE event_id = 'done'`); err == nil {
		t.Fatal("seq accepted NULL; want NOT NULL")
	}
	ob, err := outbox.NewStore(pool, OutboxTable)
	if err != nil {
		t.Fatal(err)
	}
	if err := sdkpgx.WithTx(ctx, pool, func(tx pgx.Tx) error {
		return ob.EnqueueTx(ctx, tx, outbox.Row{EventID: "a4", AggregateType: "ChatThread", AggregateID: "thread-A", EventType: "T", Payload: []byte("a4")})
	}); err != nil {
		t.Fatalf("enqueue after upgrade: %v", err)
	}
	var maxOld, newSeq int64
	if err := pool.QueryRow(ctx, `SELECT (SELECT max(seq) FROM chat_outbox_events WHERE event_id <> 'a4'), (SELECT seq FROM chat_outbox_events WHERE event_id = 'a4')`).Scan(&maxOld, &newSeq); err != nil || newSeq <= maxOld {
		t.Fatalf("new row seq = %d, max old seq = %d, err = %v", newSeq, maxOld, err)
	}

	// Indexes: the v1 ones exist, the v0 claim index is gone.
	idx := map[string]bool{}
	irows, err := pool.Query(ctx, `SELECT indexname FROM pg_indexes WHERE tablename = 'chat_outbox_events'`)
	if err != nil {
		t.Fatal(err)
	}
	for irows.Next() {
		var n string
		if err := irows.Scan(&n); err != nil {
			t.Fatal(err)
		}
		idx[n] = true
	}
	irows.Close()
	for _, n := range []string{"chat_outbox_events_status_idx", "chat_outbox_events_agg_idx", "chat_outbox_events_pending_idx"} {
		if !idx[n] {
			t.Errorf("index %s missing; have %v", n, idx)
		}
	}
	if idx["idx_chat_outbox_claim"] {
		t.Error("v0 claim index idx_chat_outbox_claim still present")
	}
	var invalid int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM pg_index i JOIN pg_class c ON c.oid = i.indexrelid WHERE NOT i.indisvalid AND c.relname LIKE 'chat_outbox_events%'`).Scan(&invalid); err != nil || invalid != 0 {
		t.Fatalf("invalid indexes = %d, err = %v", invalid, err)
	}

	// The SDK relayer publishes pending rows per aggregate in creation order and
	// does not resend the published one.
	rec := testkit.NewRecorder()
	relayer, err := outbox.NewRelayer(ob, rec, outbox.RelayerOptions{
		Topic:  "chat.events",
		Config: sdkconfig.Outbox{BatchSize: 10, ClaimLock: time.Minute},
	})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ { // a1 was retried with available_at = now(), so a pass may need to wait for it
		if _, err := relayer.RelayOnce(ctx); err != nil {
			t.Fatalf("relay: %v", err)
		}
	}
	perAgg := map[string][]string{}
	for _, r := range rec.Records() {
		perAgg[string(r.Key)] = append(perAgg[string(r.Key)], string(r.Value))
	}
	if want := []string{"a1", "a2", "a3", "a4"}; !slices.Equal(perAgg["thread-A"], want) {
		t.Fatalf("thread-A published %v, want %v (creation order, published row not resent)", perAgg["thread-A"], want)
	}
	if want := []string{"b1"}; !slices.Equal(perAgg["thread-B"], want) {
		t.Fatalf("thread-B published %v, want %v", perAgg["thread-B"], want)
	}
	if strings.Contains(strings.Join(perAgg["thread-A"], ","), "done") {
		t.Fatal("published row was resent")
	}
}
