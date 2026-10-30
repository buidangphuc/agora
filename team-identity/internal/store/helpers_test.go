package store_test

import (
	"context"
	"io/fs"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/migrate"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"

	"github.com/buidangphuc/team-identity/internal/store"
)

const latestMigration = 19

func schemaDSN(t testing.TB) string {
	t.Helper()
	return testkit.Schema(t, testkit.SharedPostgres(t))
}

func migrationsUpTo(t testing.TB, version int) fs.FS {
	t.Helper()
	src := store.Migrations()
	entries, err := fs.ReadDir(src, ".")
	if err != nil {
		t.Fatalf("read embedded migrations: %v", err)
	}
	out := fstest.MapFS{}
	for _, e := range entries {
		name := e.Name()
		v, err := strconv.Atoi(strings.SplitN(name, "_", 2)[0])
		if err != nil || v > version {
			continue
		}
		b, err := fs.ReadFile(src, name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		out[name] = &fstest.MapFile{Data: b}
	}
	return out
}

func migrateTo(t testing.TB, dsn string, version int) {
	t.Helper()
	if err := migrate.Up(context.Background(), dsn, migrationsUpTo(t, version)); err != nil {
		t.Fatalf("migrate to %d: %v", version, err)
	}
}

func openPool(t testing.TB, dsn string) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func poolAt(t testing.TB, version int) *pgxpool.Pool {
	t.Helper()
	dsn := schemaDSN(t)
	migrateTo(t, dsn, version)
	return openPool(t, dsn)
}

func latestPool(t testing.TB) *pgxpool.Pool {
	t.Helper()
	return poolAt(t, latestMigration)
}

func migrationSQL(t testing.TB, name string) string {
	t.Helper()
	b, err := fs.ReadFile(store.Migrations(), name)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(b)
}

const outboxTable = "identity_outbox_events"

func relayRecords(t testing.TB, pool *pgxpool.Pool, want int) []testkit.Record {
	t.Helper()
	ob, err := store.Outbox(pool)
	if err != nil {
		t.Fatalf("outbox store: %v", err)
	}
	rec := testkit.NewRecorder()
	relayer, err := outbox.NewRelayer(ob, rec, outbox.RelayerOptions{
		Topic:  "identity.events",
		Config: config.Outbox{BatchSize: 10, ClaimLock: time.Minute},
	})
	if err != nil {
		t.Fatalf("relayer: %v", err)
	}
	for i := 0; i < 10 && len(rec.Records()) < want; i++ {
		if _, err := relayer.RelayOnce(context.Background()); err != nil {
			t.Fatalf("relay once: %v", err)
		}
	}
	return rec.Records()
}

func relayValues(t testing.TB, pool *pgxpool.Pool, want int) []string {
	t.Helper()
	var out []string
	for _, r := range relayRecords(t, pool, want) {
		out = append(out, string(r.Value))
	}
	return out
}

func enqueueUserRow(t testing.TB, pool *pgxpool.Pool, row outbox.Row) {
	t.Helper()
	ctx := context.Background()
	ob, err := store.Outbox(pool)
	if err != nil {
		t.Fatalf("outbox store: %v", err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if err := ob.EnqueueTx(ctx, tx, row); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("enqueue %s: %v", row.EventID, err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}
}
