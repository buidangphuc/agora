package migrations_test

import (
	"io/fs"
	"os"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/migrate"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"

	"github.com/buidangphuc/team-order/migrations"
)

const codBackfillUp = "0024_listing_sales_cod.up.sql"

func newMigDB0024(t *testing.T) *mdb {
	t.Helper()
	dsn := testkit.Schema(t, testkit.SharedPostgres(t))
	pool, err := pgxpool.New(t.Context(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	before := fstest.MapFS{}
	entries, err := fs.ReadDir(migrations.FS, ".")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() >= "0024" {
			continue
		}
		b, err := fs.ReadFile(migrations.FS, e.Name())
		if err != nil {
			t.Fatal(err)
		}
		before[e.Name()] = &fstest.MapFile{Data: b}
	}
	if err := migrate.Up(t.Context(), dsn, before); err != nil {
		t.Fatalf("migrate to 0023: %v", err)
	}
	m := &mdb{t: t, ctx: t.Context(), pool: pool}
	return m
}

func (m *mdb) runCodUp() time.Time {
	m.t.Helper()
	b, err := os.ReadFile(codBackfillUp)
	if err != nil {
		m.t.Fatalf("read %s: %v", codBackfillUp, err)
	}
	tx, err := m.pool.Begin(m.ctx)
	if err != nil {
		m.t.Fatal(err)
	}
	if _, err := tx.Exec(m.ctx, string(b)); err != nil {
		_ = tx.Rollback(m.ctx)
		m.t.Fatalf("apply %s: %v", codBackfillUp, err)
	}
	var txTime time.Time
	if err := tx.QueryRow(m.ctx, `SELECT now()`).Scan(&txTime); err != nil {
		m.t.Fatal(err)
	}
	if err := tx.Commit(m.ctx); err != nil {
		m.t.Fatal(err)
	}
	return txTime
}

func (m *mdb) seedSold(listing string, n int64) {
	m.t.Helper()
	m.exec(`INSERT INTO listing_sales (listing_id, sold_count) VALUES ($1, $2)
		ON CONFLICT (listing_id) DO UPDATE SET sold_count = EXCLUDED.sold_count`, listing, n)
}

func (m *mdb) seedLedger(orderID, listing string, qty int) {
	m.t.Helper()
	m.exec(`INSERT INTO listing_sales_orders (order_id, listing_id, quantity, state) VALUES ($1, $2, $3, 'counted')`,
		orderID, listing, qty)
}

func (m *mdb) outboxRows() int {
	m.t.Helper()
	return m.count(`SELECT count(*) FROM order_outbox_events`)
}

func TestMigration0024_HistoricalCodCompletedOrdersAreCountedOnce(t *testing.T) {
	m := newMigDB0024(t)
	m.seedSold("L", 3)
	m.order("cod-done", stCompleted, mCOD, false)
	m.line("cod-done", "L", 2)
	m.order("cod-shipped", stShipped, mCOD, false)
	m.line("cod-shipped", "L", 4)
	m.order("cod-pending", stPending, mCOD, false)
	m.line("cod-pending", "L", 1)

	m.runCodUp()

	m.wantSold("L", 5)
	if n := m.count(`SELECT count(*) FROM listing_sales_orders`); n != 1 {
		t.Fatalf("ledger rows = %d, want 1", n)
	}
	if s := m.ledgerState("cod-done", "L"); s != "counted" {
		t.Fatalf("ledger state of the COD COMPLETED order = %q, want counted", s)
	}
	if n := m.count(`SELECT count(*) FROM listing_sales_orders WHERE order_id = 'cod-done' AND quantity = 2`); n != 1 {
		t.Fatal("ledger quantity of the COD COMPLETED order must be 2")
	}
	if n := m.outboxRows(); n != 1 {
		t.Fatalf("outbox rows = %d, want 1", n)
	}
	m.wantEventValue("L", 5)
	row := m.salesEvents()["L"]
	env, _ := decodeSales(t, row)
	if row.EventID == "" || env.GetEventId() != row.EventID {
		t.Fatalf("row event id %q, envelope event id %q", row.EventID, env.GetEventId())
	}
	var legacy string
	if err := m.pool.QueryRow(m.ctx, `SELECT md5('listing-sales:' || 'L' || ':' || 5::text || ':backfill')::uuid::text`).Scan(&legacy); err != nil {
		t.Fatal(err)
	}
	if row.EventID == legacy {
		t.Fatalf("event id %q collides with the 0023 backfill id", row.EventID)
	}
}

func TestMigration0024_IgnoresLedgerAndNonCodOrders(t *testing.T) {
	m := newMigDB0024(t)
	m.seedSold("L", 3)
	m.order("momo-done", stCompleted, mMomo, true)
	m.line("momo-done", "L", 1)
	m.seedLedger("momo-done", "L", 1)
	m.order("cod-done", stCompleted, mCOD, false)
	m.line("cod-done", "L", 2)
	m.seedLedger("cod-done", "L", 2)
	m.order("momo-done-unledgered", stCompleted, mMomo, true)
	m.line("momo-done-unledgered", "L", 7)
	ledgerBefore := m.count(`SELECT count(*) FROM listing_sales_orders`)

	m.runCodUp()

	m.wantSold("L", 3)
	if n := m.count(`SELECT count(*) FROM listing_sales_orders`); n != ledgerBefore {
		t.Fatalf("ledger rows %d -> %d", ledgerBefore, n)
	}
	if n := m.outboxRows(); n != 0 {
		t.Fatalf("outbox rows = %d, want 0", n)
	}
}

func TestMigration0024_BackfillTwiceDoesNotDuplicate(t *testing.T) {
	m := newMigDB0024(t)
	m.seedSold("L", 3)
	m.order("cod-a", stCompleted, mCOD, false)
	m.line("cod-a", "L", 2)
	m.order("cod-b", stCompleted, mCOD, false)
	m.line("cod-b", "M", 4)

	m.runCodUp()
	first := m.salesEvents()
	ledgerRows := m.count(`SELECT count(*) FROM listing_sales_orders`)
	outboxRows := m.outboxRows()
	if ledgerRows != 2 || outboxRows != 2 {
		t.Fatalf("after the first run: ledger %d outbox %d, want 2 and 2", ledgerRows, outboxRows)
	}

	m.runCodUp()

	m.wantSold("L", 5)
	m.wantSold("M", 4)
	if n := m.count(`SELECT count(*) FROM listing_sales`); n != 2 {
		t.Fatalf("listing_sales rows = %d, want 2", n)
	}
	if n := m.count(`SELECT count(*) FROM listing_sales_orders`); n != ledgerRows {
		t.Fatalf("ledger rows %d -> %d", ledgerRows, n)
	}
	if n := m.outboxRows(); n != outboxRows {
		t.Fatalf("outbox rows %d -> %d", outboxRows, n)
	}
	for listing, row := range first {
		if m.salesEvents()[listing].EventID != row.EventID {
			t.Fatalf("event id of %s changed between runs", listing)
		}
	}
}

func TestMigration0024_NoCodOrders(t *testing.T) {
	m := newMigDB0024(t)
	m.seedSold("L", 3)
	m.order("momo-paid", stPaid, mMomo, true)
	m.line("momo-paid", "L", 2)
	m.order("cod-shipped", stShipped, mCOD, false)
	m.line("cod-shipped", "L", 1)

	m.runCodUp()

	m.wantSold("L", 3)
	if n := m.count(`SELECT count(*) FROM listing_sales`); n != 1 {
		t.Fatalf("listing_sales rows = %d, want 1", n)
	}
	if n := m.count(`SELECT count(*) FROM listing_sales_orders`); n != 0 {
		t.Fatalf("ledger rows = %d, want 0", n)
	}
	if n := m.outboxRows(); n != 0 {
		t.Fatalf("outbox rows = %d, want 0", n)
	}
}
