package pgx_test

import (
	"context"
	"errors"
	"testing"
	"time"

	jpgx "github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"
	sdkpgx "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/pgx"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/testkit"
)

func openPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, err := sdkpgx.Open(context.Background(), config.Postgres{URL: testkit.Postgres(t), MaxConns: 4})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(context.Background(), "CREATE TABLE items (id int PRIMARY KEY)"); err != nil {
		t.Fatalf("create table: %v", err)
	}
	return pool
}

func countItems(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM items").Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	return n
}

func TestOpenFailsOnUnreachableDB(t *testing.T) {
	start := time.Now()
	pool, err := sdkpgx.Open(context.Background(), config.Postgres{
		URL:      "postgres://u:p@127.0.0.1:1/db?sslmode=disable",
		MaxConns: 2,
	})
	if err == nil {
		pool.Close()
		t.Fatal("expected error for unreachable database")
	}
	if pool != nil {
		t.Fatal("pool must be nil on error")
	}
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Fatalf("Open took %s, want a bounded ping", elapsed)
	}
}

func TestWithTxRollsBackOnError(t *testing.T) {
	pool := openPool(t)
	boom := errors.New("boom")

	err := sdkpgx.WithTx(context.Background(), pool, func(tx jpgx.Tx) error {
		if _, err := tx.Exec(context.Background(), "INSERT INTO items VALUES (1)"); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
	if n := countItems(t, pool); n != 0 {
		t.Fatalf("rows after rollback = %d, want 0", n)
	}
}

func TestWithTxCommits(t *testing.T) {
	pool := openPool(t)

	err := sdkpgx.WithTx(context.Background(), pool, func(tx jpgx.Tx) error {
		_, err := tx.Exec(context.Background(), "INSERT INTO items VALUES (1)")
		return err
	})
	if err != nil {
		t.Fatalf("WithTx: %v", err)
	}
	if n := countItems(t, pool); n != 1 {
		t.Fatalf("rows after commit = %d, want 1", n)
	}
	if err := sdkpgx.Checker(pool)(context.Background()); err != nil {
		t.Fatalf("Checker: %v", err)
	}
}
