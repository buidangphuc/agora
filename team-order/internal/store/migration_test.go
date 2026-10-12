package store_test

import (
	"context"
	"errors"
	"io/fs"
	"sort"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/consume"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/testkit"

	"github.com/buidangphuc/team-order/migrations"
)

const paymentConsumer = "team-order.payment"

// execMigrations runs the .up.sql files with a number in (after, upto], in order.
func execMigrations(t *testing.T, pool *pgxpool.Pool, after, upto string) {
	t.Helper()
	entries, err := fs.ReadDir(migrations.Files, ".")
	if err != nil {
		t.Fatalf("read embedded migrations: %v", err)
	}
	var names []string
	for _, e := range entries {
		n := e.Name()
		if strings.HasSuffix(n, ".up.sql") && n[:4] > after && n[:4] <= upto {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	for _, n := range names {
		sqlBytes, err := fs.ReadFile(migrations.Files, n)
		if err != nil {
			t.Fatalf("read %s: %v", n, err)
		}
		if _, err := pool.Exec(context.Background(), string(sqlBytes)); err != nil {
			t.Fatalf("apply %s: %v", n, err)
		}
	}
}

func execDown(t *testing.T, pool *pgxpool.Pool, name string) {
	t.Helper()
	sqlBytes, err := fs.ReadFile(migrations.Files, name)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	if _, err := pool.Exec(context.Background(), string(sqlBytes)); err != nil {
		t.Fatalf("apply %s: %v", name, err)
	}
}

func newPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), testkit.Postgres(t))
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func seen(t *testing.T, pool *pgxpool.Pool, consumerName, eventID string) bool {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	already, err := consume.NewDedupe("").Seen(ctx, tx, consumerName, eventID)
	if err != nil {
		t.Fatalf("Seen(%s, %s): %v", consumerName, eventID, err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return already
}

// A row processed before 0008 (keyed by event_id alone, consumer empty or set) is
// recognised afterwards under the consumer team-order.payment, so a redelivered
// event is not applied twice.
func TestMigration0008KeepsExistingDedupe(t *testing.T) {
	ctx := context.Background()
	pool := newPool(t)
	execMigrations(t, pool, "0000", "0007")

	for _, row := range [][2]string{{"evt-legacy-empty", ""}, {"evt-legacy-named", paymentConsumer}} {
		if _, err := pool.Exec(ctx, `INSERT INTO processed_events (event_id, consumer) VALUES ($1, $2)`, row[0], row[1]); err != nil {
			t.Fatalf("seed legacy row %v: %v", row, err)
		}
	}

	execMigrations(t, pool, "0007", "0008")

	if !seen(t, pool, paymentConsumer, "evt-legacy-empty") {
		t.Error("a legacy row with an empty consumer must count as processed by team-order.payment")
	}
	if !seen(t, pool, paymentConsumer, "evt-legacy-named") {
		t.Error("a legacy row already named team-order.payment must still count as processed")
	}
	if seen(t, pool, paymentConsumer, "evt-new") {
		t.Error("an unseen event must not be reported as processed")
	}
	var empty int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM processed_events WHERE consumer = ''`).Scan(&empty); err != nil || empty != 0 {
		t.Fatalf("rows left with an empty consumer = %d (err %v), want 0", empty, err)
	}
	// The key is (consumer, event_id): another consumer may process the same event.
	if seen(t, pool, "another.consumer", "evt-legacy-named") {
		t.Error("the same event id must be independent per consumer")
	}
}

// Down restores the old key on event_id alone, keeping one row per event.
func TestMigration0008DownRestoresEventIDKey(t *testing.T) {
	ctx := context.Background()
	pool := newPool(t)
	execMigrations(t, pool, "0000", "0008")
	if seen(t, pool, paymentConsumer, "evt-1") || seen(t, pool, "another.consumer", "evt-1") {
		t.Fatal("fresh ledger reported a duplicate")
	}

	execDown(t, pool, "0008_processed_events_consumer_pk.down.sql")

	var rows int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM processed_events WHERE event_id = 'evt-1'`).Scan(&rows); err != nil || rows != 1 {
		t.Fatalf("rows for evt-1 after down = %d (err %v), want 1", rows, err)
	}
	_, err := pool.Exec(ctx, `INSERT INTO processed_events (event_id, consumer) VALUES ('evt-1', 'third.consumer')`)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		t.Fatalf("insert of a second consumer for the same event = %v, want a unique violation (old key restored)", err)
	}
}
