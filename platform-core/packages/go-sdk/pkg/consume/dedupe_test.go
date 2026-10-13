package consume_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/consume"
	sdkpgx "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/pgx"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/testkit"
)

func dedupePool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, err := sdkpgx.Open(context.Background(), config.Postgres{URL: testkit.Postgres(t), MaxConns: 4})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(pool.Close)
	ddl, err := consume.DedupeSchemaSQL("processed_events")
	if err != nil {
		t.Fatalf("schema sql: %v", err)
	}
	if _, err := pool.Exec(context.Background(), ddl); err != nil {
		t.Fatalf("schema: %v", err)
	}
	return pool
}

func seen(t *testing.T, pool *pgxpool.Pool, d consume.Dedupe, consumer, id string) bool {
	t.Helper()
	tx, err := pool.Begin(context.Background())
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	s, err := d.Seen(context.Background(), tx, consumer, id)
	if err != nil {
		t.Fatalf("seen: %v", err)
	}
	if err := tx.Commit(context.Background()); err != nil {
		t.Fatalf("commit: %v", err)
	}
	return s
}

func TestDedupeIsPerConsumer(t *testing.T) {
	pool := dedupePool(t)
	d := newDedupe(t, "")

	if seen(t, pool, d, "billing", "e1") {
		t.Fatal("first delivery to billing reported seen")
	}
	if !seen(t, pool, d, "billing", "e1") {
		t.Fatal("second delivery to billing not reported seen")
	}
	if seen(t, pool, d, "email", "e1") {
		t.Fatal("same event for another consumer must not be seen")
	}
}

func TestDedupeRollsBackWithTx(t *testing.T) {
	pool := dedupePool(t)
	d := newDedupe(t, "")

	tx, err := pool.Begin(context.Background())
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if s, err := d.Seen(context.Background(), tx, "billing", "e1"); err != nil || s {
		t.Fatalf("seen = %v, err = %v", s, err)
	}
	if err := tx.Rollback(context.Background()); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if seen(t, pool, d, "billing", "e1") {
		t.Fatal("rolled-back handling must leave the event unseen")
	}
}

func TestDedupeProcessedIsReadOnly(t *testing.T) {
	ctx := context.Background()
	pool := dedupePool(t)
	d := newDedupe(t, "")

	if seen(t, pool, d, "a", "e1") {
		t.Fatal("first mark reported seen")
	}
	check := func(q consume.Querier, consumer, id string, want bool) {
		t.Helper()
		got, err := d.Processed(ctx, q, consumer, id)
		if err != nil {
			t.Fatalf("Processed(%s, %s): %v", consumer, id, err)
		}
		if got != want {
			t.Fatalf("Processed(%s, %s) = %v, want %v", consumer, id, got, want)
		}
	}
	check(pool, "a", "e1", true)
	check(pool, "b", "e1", false)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	check(tx, "a", "e1", true)
	check(tx, "b", "e1", false)
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback: %v", err)
	}

	var n int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM processed_events").Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Fatalf("rows = %d, want 1: Processed must not insert", n)
	}
}

func TestDedupeProcessedReturnsErrors(t *testing.T) {
	pool := dedupePool(t)

	if _, err := newDedupe(t, "missing_table").Processed(context.Background(), pool, "a", "e1"); err == nil {
		t.Fatal("missing table: want an error")
	}
}

func newDedupe(t *testing.T, table string) consume.Dedupe {
	t.Helper()
	d, err := consume.NewDedupe(table)
	if err != nil {
		t.Fatalf("NewDedupe(%q): %v", table, err)
	}
	return d
}

func TestNewDedupeValidatesTable(t *testing.T) {
	for _, bad := range []string{"bad name;", "1abc", "Upper", "a-b", "x; DROP TABLE y"} {
		if _, err := consume.NewDedupe(bad); err == nil {
			t.Errorf("NewDedupe(%q) = nil error, want invalid table", bad)
		}
		if _, err := consume.DedupeSchemaSQL(bad); err == nil {
			t.Errorf("DedupeSchemaSQL(%q) = nil error, want invalid table", bad)
		}
	}
	for _, ok := range []string{"", "processed_events", "_t1"} {
		if _, err := consume.NewDedupe(ok); err != nil {
			t.Errorf("NewDedupe(%q): %v", ok, err)
		}
	}
}

type fakeExec struct {
	sql  string
	args []any
	tag  pgconn.CommandTag
	err  error
}

func (f *fakeExec) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	f.sql, f.args = sql, args
	return f.tag, f.err
}

func TestDedupeSeenAcceptsAnyExecutor(t *testing.T) {
	d := newDedupe(t, "")
	ex := &fakeExec{tag: pgconn.NewCommandTag("INSERT 0 1")}
	seen, err := d.Seen(context.Background(), ex, "billing", "e1")
	if err != nil || seen {
		t.Fatalf("first insert: seen = %v, err = %v", seen, err)
	}
	if !strings.Contains(ex.sql, "processed_events") {
		t.Fatalf("zero-value table not defaulted: %q", ex.sql)
	}
	ex.tag = pgconn.NewCommandTag("INSERT 0 0")
	if seen, _ := d.Seen(context.Background(), ex, "billing", "e1"); !seen {
		t.Fatal("conflict must report seen")
	}
	ex.err = errors.New("db down")
	if _, err := d.Seen(context.Background(), ex, "billing", "e1"); !errors.Is(err, ex.err) {
		t.Fatalf("err = %v, want the executor error", err)
	}
}
