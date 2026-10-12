package consume_test

import (
	"context"
	"testing"

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
	if _, err := pool.Exec(context.Background(), consume.DedupeSchemaSQL("processed_events")); err != nil {
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
	d := consume.NewDedupe("")

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
	d := consume.NewDedupe("")

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
