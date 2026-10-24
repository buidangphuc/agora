package store_test

import (
	"io/fs"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/migrate"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"
	"github.com/buidangphuc/team-promotion/internal/store"
)

const outboxTable = "promotion_outbox"

const (
	voucherChanged   = "platform.promotion.v1.VoucherChanged"
	flashSaleChanged = "platform.promotion.v1.FlashSaleChanged"
)

func schemaDSN(t *testing.T) string {
	t.Helper()
	server := os.Getenv("PROMOTION_TEST_DATABASE_URL")
	if server == "" {
		server = testkit.SharedPostgres(t)
	}
	return testkit.Schema(t, server)
}

func openPool(t *testing.T, dsn string) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(t.Context(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func migrationsFS(t *testing.T) fs.FS {
	t.Helper()
	sub, err := fs.Sub(store.Migrations, "migrations")
	if err != nil {
		t.Fatalf("sub migrations: %v", err)
	}
	return sub
}

func migrationSQL(t *testing.T, name string) string {
	t.Helper()
	body, err := fs.ReadFile(store.Migrations, "migrations/"+name)
	if err != nil {
		t.Fatalf("read embedded migration %s: %v", name, err)
	}
	return string(body)
}

func newTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := schemaDSN(t)
	if err := migrate.Up(t.Context(), dsn, migrationsFS(t)); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	return openPool(t, dsn)
}

func newPoolUpTo(t *testing.T, version int) *pgxpool.Pool {
	t.Helper()
	pool := openPool(t, schemaDSN(t))
	entries, err := fs.ReadDir(store.Migrations, "migrations")
	if err != nil {
		t.Fatalf("list embedded migrations: %v", err)
	}
	applied := 0
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".up.sql") {
			continue
		}
		n, err := strconv.Atoi(strings.SplitN(name, "_", 2)[0])
		if err != nil {
			t.Fatalf("migration %s has no numeric prefix", name)
		}
		if n > version {
			continue
		}
		if _, err := pool.Exec(t.Context(), migrationSQL(t, name)); err != nil {
			t.Fatalf("apply %s: %v", name, err)
		}
		applied++
	}
	if applied != version {
		t.Fatalf("applied %d migrations up to %d, want %d", applied, version, version)
	}
	return pool
}

func newOutbox(t *testing.T, pool *pgxpool.Pool) *outbox.Store {
	t.Helper()
	ob, err := outbox.NewStore(pool, outboxTable)
	if err != nil {
		t.Fatalf("outbox store: %v", err)
	}
	return ob
}

func voucherEvent(v store.Voucher) (outbox.Row, error) {
	return outbox.Row{
		EventID:       "ev-voucher-" + v.ID,
		AggregateType: "voucher",
		AggregateID:   v.ID,
		EventType:     voucherChanged,
		Payload:       []byte("voucher-payload"),
		Topic:         "promotion.events",
	}, nil
}

func campaignEvent(c store.FlashSaleCampaign) (outbox.Row, error) {
	return outbox.Row{
		EventID:       "ev-campaign-" + c.ID,
		AggregateType: "flash_sale",
		AggregateID:   c.ID,
		EventType:     flashSaleChanged,
		Payload:       []byte("campaign-payload"),
		Topic:         "promotion.events",
	}, nil
}

func outboxCount(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM `+outboxTable).Scan(&n); err != nil {
		t.Fatalf("count outbox: %v", err)
	}
	return n
}
