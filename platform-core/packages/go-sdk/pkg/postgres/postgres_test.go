package postgres_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	pgx "github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/postgres"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/testkit"
)

func openPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, err := postgres.Open(context.Background(), config.Postgres{URL: testkit.Postgres(t), MaxConns: 4})
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
	pool, err := postgres.Open(context.Background(), config.Postgres{
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

	err := postgres.WithTx(context.Background(), pool, func(tx pgx.Tx) error {
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

	err := postgres.WithTx(context.Background(), pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), "INSERT INTO items VALUES (1)")
		return err
	})
	if err != nil {
		t.Fatalf("WithTx: %v", err)
	}
	if n := countItems(t, pool); n != 1 {
		t.Fatalf("rows after commit = %d, want 1", n)
	}
	if err := postgres.Checker(pool)(context.Background()); err != nil {
		t.Fatalf("Checker: %v", err)
	}
}

func TestOpenOrLocalFallsBackOnlyLocally(t *testing.T) {
	c := config.Postgres{URL: "postgres://u:p@127.0.0.1:1/db?connect_timeout=1", MaxConns: 2}
	logger := slog.New(slog.DiscardHandler)

	pool, err := postgres.OpenOrLocal(context.Background(), c, config.Runtime{Env: "dev"}, logger)
	if err == nil || pool != nil {
		t.Errorf("dev: pool=%v err=%v; want nil pool and an error", pool, err)
	}
	for _, env := range []string{"local", "test"} {
		pool, err = postgres.OpenOrLocal(context.Background(), c, config.Runtime{Env: env}, logger)
		if err != nil || pool != nil {
			t.Errorf("%s: pool=%v err=%v; want nil, nil", env, pool, err)
		}
	}
}

func TestWithTxPanicDoesNotLeakConnection(t *testing.T) {
	pool := openPool(t) // MaxConns: 4

	for i := 0; i < 8; i++ { // more panics than connections
		func() {
			defer func() {
				if r := recover(); r == nil {
					t.Error("WithTx must re-panic (or the pool is exhausted)")
				}
			}()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = postgres.WithTx(ctx, pool, func(tx pgx.Tx) error {
				if _, err := tx.Exec(ctx, "INSERT INTO items VALUES (1)"); err != nil {
					return err
				}
				panic("boom")
			})
		}()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("pool exhausted after panics: %v", err)
	}
	if n := countItems(t, pool); n != 0 {
		t.Fatalf("rows after panics = %d, want 0", n)
	}
	if got := pool.Stat().AcquiredConns(); got != 0 {
		t.Fatalf("acquired conns = %d, want 0", got)
	}
}

func TestOpenOrLocalNilLogger(t *testing.T) {
	c := config.Postgres{URL: "postgres://u:p@127.0.0.1:1/db?connect_timeout=1", MaxConns: 2}
	pool, err := postgres.OpenOrLocal(context.Background(), c, config.Runtime{Env: "local"}, nil)
	if err != nil || pool != nil {
		t.Fatalf("pool=%v err=%v; want nil, nil", pool, err)
	}
}
