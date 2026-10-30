package store_test

import (
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/migrate"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"
)

const outboxTable = "order_outbox_events"

func newDatabase(t *testing.T) (string, *pgxpool.Pool) {
	t.Helper()
	dsn := testkit.Schema(t, testkit.SharedPostgres(t))
	pool, err := pgxpool.New(t.Context(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return dsn, pool
}

func pgPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn, pool := newDatabase(t)
	if err := migrate.Up(t.Context(), dsn, os.DirFS("../../migrations")); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	return pool
}

func newOutbox(t *testing.T, pool *pgxpool.Pool) *outbox.Store {
	t.Helper()
	s, err := outbox.NewStore(pool, outboxTable)
	if err != nil {
		t.Fatalf("outbox store: %v", err)
	}
	return s
}

func uid(prefix string) string { return prefix + "-" + uuid.NewString() }
