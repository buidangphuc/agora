package migrations_test

import (
	"context"
	"io/fs"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/kafkax"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/migrate"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/testkit"

	"github.com/buidangphuc/team-identity/migrations"
)

const table = "identity_outbox_events"

// upTo returns the migrations whose file name sorts at or below last, so a test
// can build the schema a service had before the outbox upgrade.
func upTo(t *testing.T, last string) fs.FS {
	t.Helper()
	out := fstest.MapFS{}
	entries, err := fs.ReadDir(migrations.Files, ".")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".sql") && e.Name()[:4] <= last {
			b, err := fs.ReadFile(migrations.Files, e.Name())
			if err != nil {
				t.Fatal(err)
			}
			out[e.Name()] = &fstest.MapFile{Data: b}
		}
	}
	return out
}

func eventIDs(recs []testkit.Record) []string {
	var ids []string
	for _, r := range recs {
		for _, h := range r.Headers {
			if h.Key == kafkax.HeaderEventID {
				ids = append(ids, string(h.Value))
			}
		}
	}
	return ids
}

// A table in the v0 schema holding rows (one aggregate with a retried row that a
// later update moved behind newer events in the heap, another aggregate, and a
// published row) is brought to the v1 schema by the migrations: seq follows
// (created_at, event_id), the v1 indexes exist and are valid, claim_idx is gone,
// and the SDK relayer publishes each aggregate in creation order.
func TestOutboxUpgradeKeepsOrderOfExistingRows(t *testing.T) {
	ctx := context.Background()
	url := testkit.Postgres(t)
	if err := migrate.Up(ctx, url, upTo(t, "0005")); err != nil {
		t.Fatalf("migrate to the v0 schema: %v", err)
	}
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	insert := func(id, agg string, at time.Duration, status string) {
		t.Helper()
		_, err := pool.Exec(ctx, `INSERT INTO `+table+`
			(event_id, aggregate_type, aggregate_id, event_type, payload, status, created_at)
			VALUES ($1, 'Session', $2, 'T', $3, $4, $5)`, id, agg, []byte(id), status, base.Add(at))
		if err != nil {
			t.Fatalf("insert %s: %v", id, err)
		}
	}
	// Created in this order within aggregate u1: e1, e2, e3. u2 has f1 between.
	insert("e1", "u1", 1*time.Second, "pending")
	insert("f1", "u2", 2*time.Second, "pending")
	insert("e2", "u1", 3*time.Second, "pending")
	insert("e3", "u1", 4*time.Second, "pending")
	insert("p1", "u3", 5*time.Second, "published")
	// A v0 relayer retried e1: the update rewrites its tuple, so heap order is now
	// e2, e3, e1 and a BIGSERIAL numbering by heap order would put e1 last.
	if _, err := pool.Exec(ctx, `UPDATE `+table+` SET attempts = 1, available_at = now(), error = 'boom' WHERE event_id = 'e1'`); err != nil {
		t.Fatal(err)
	}

	if err := migrate.Up(ctx, url, migrations.Files); err != nil {
		t.Fatalf("upgrade migrations: %v", err)
	}

	// seq is numbered by (created_at, event_id), not by heap order.
	rows, err := pool.Query(ctx, `SELECT event_id FROM `+table+` ORDER BY seq`)
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
	if want := []string{"e1", "f1", "e2", "e3", "p1"}; !slices.Equal(bySeq, want) {
		t.Fatalf("order by seq = %v, want %v", bySeq, want)
	}

	var notNull bool
	if err := pool.QueryRow(ctx, `SELECT attnotnull FROM pg_attribute WHERE attrelid = $1::regclass AND attname = 'seq'`, table).Scan(&notNull); err != nil || !notNull {
		t.Fatalf("seq must be NOT NULL (got %v, err %v)", notNull, err)
	}
	var stray int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM pg_constraint WHERE conrelid = $1::regclass AND conname = $2`, table, table+"_seq_nn").Scan(&stray); err != nil || stray != 0 {
		t.Fatalf("the temporary CHECK constraint must be dropped (found %d, err %v)", stray, err)
	}

	// v1 indexes exist and are valid; the v0 claim index is gone.
	for _, idx := range []string{table + "_status_idx", table + "_agg_idx", table + "_pending_idx"} {
		var valid bool
		if err := pool.QueryRow(ctx, `SELECT i.indisvalid FROM pg_index i WHERE i.indexrelid = to_regclass($1)`, idx).Scan(&valid); err != nil || !valid {
			t.Errorf("index %s must exist and be valid (valid=%v err=%v)", idx, valid, err)
		}
	}
	var claim *string
	if err := pool.QueryRow(ctx, `SELECT to_regclass($1)::text`, table+"_claim_idx").Scan(&claim); err != nil || claim != nil {
		t.Errorf("claim_idx must be dropped (got %v, err %v)", claim, err)
	}

	// A row enqueued by the SDK after the upgrade is numbered after every old row.
	store, err := outbox.NewStore(pool, table)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.EnqueueTx(ctx, tx, outbox.Row{EventID: "e4", AggregateType: "Session", AggregateID: "u1", EventType: "T", Payload: []byte("e4")}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var maxOld, e4 int64
	if err := pool.QueryRow(ctx, `SELECT max(seq) FILTER (WHERE event_id <> 'e4'), max(seq) FILTER (WHERE event_id = 'e4') FROM `+table).Scan(&maxOld, &e4); err != nil || e4 <= maxOld {
		t.Fatalf("new row seq %d must follow old rows (max %d), err %v", e4, maxOld, err)
	}

	// The SDK relayer publishes u1 in creation order, the retried row first.
	rec := testkit.NewRecorder()
	relayer, err := outbox.NewRelayer(store, rec, outbox.RelayerOptions{
		Topic:  "identity.events",
		Config: config.Outbox{BatchSize: 10, ClaimLock: time.Minute},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := relayer.RelayOnce(ctx); err != nil {
		t.Fatalf("RelayOnce: %v", err)
	}
	var u1 []string
	for _, r := range rec.Records() {
		if string(r.Key) == "u1" {
			u1 = append(u1, eventIDs([]testkit.Record{r})...)
		}
	}
	if want := []string{"e1", "e2", "e3", "e4"}; !slices.Equal(u1, want) {
		t.Fatalf("u1 published as %v, want %v", u1, want)
	}
}
