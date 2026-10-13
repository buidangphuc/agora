package ga4relay

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/buidangphuc/team-analytics/internal/store/duckdb"
)

// Required interface (implementer provides, in package ga4relay):
//
//	type Purger interface{ Purge(ctx context.Context) (int64, error) }
//
// Every MappingStore / Ledger returned by the New* constructors (memory and SQL) must also
// satisfy Purger (type assertion), so the existing interfaces stay unchanged.
// Purge physically deletes rows whose expiry <= now(), returns the number deleted.

func asPurger(t *testing.T, v any) Purger {
	t.Helper()
	p, ok := v.(Purger)
	if !ok {
		t.Fatalf("%T does not implement Purger", v)
	}
	return p
}

func countRows(t *testing.T, db *sql.DB, table string) int {
	t.Helper()
	var n int
	if err := db.QueryRow("SELECT count(*) FROM " + table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func openDB(t *testing.T) *sql.DB {
	t.Helper()
	w, err := duckdb.Open(context.Background(), filepath.Join(t.TempDir(), "wh.duckdb"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = w.Close() })
	return w.DB()
}

func purgeCheck(t *testing.T, p Purger, wantDeleted int64) {
	t.Helper()
	n, err := p.Purge(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if n != wantDeleted {
		t.Fatalf("Purge deleted %d, want %d", n, wantDeleted)
	}
}

// Scenario: Mapping expires (30 d retention) - expired rows are physically removed.
func TestPurge_MappingStore_DuckDB_DeletesExpiredRowsKeepsFresh(t *testing.T) {
	ctx := context.Background()
	clk := &fakeClock{t: time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)}
	db := openDB(t)
	s, err := NewSQLMappingStore(ctx, db, clk.Now)
	if err != nil {
		t.Fatal(err)
	}
	p := asPurger(t, s)
	_ = s.Put(ctx, "old1", "1.1")
	_ = s.Put(ctx, "old2", "2.2")
	clk.Advance(20 * 24 * time.Hour)
	_ = s.Put(ctx, "fresh", "3.3")
	purgeCheck(t, p, 0) // nothing expired yet
	if got := countRows(t, db, "ga4_relay_client_map"); got != 3 {
		t.Fatalf("rows=%d want 3", got)
	}
	clk.Advance(10*24*time.Hour + time.Second) // old* now past 30d, fresh at 10d
	purgeCheck(t, p, 2)
	if got := countRows(t, db, "ga4_relay_client_map"); got != 1 {
		t.Fatalf("rows after purge=%d want 1", got)
	}
	if id, ok := mustGet(t.Context(), t, s, "fresh"); !ok || id != "3.3" {
		t.Fatalf("fresh mapping lost: %q,%v", id, ok)
	}
	purgeCheck(t, p, 0) // idempotent
}

// Scenario: Mapping expires - boundary: a row with expiry == now is expired and is purged.
func TestPurge_MappingStore_DuckDB_ExpiryBoundaryInclusive(t *testing.T) {
	ctx := context.Background()
	clk := &fakeClock{t: time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)}
	db := openDB(t)
	s, _ := NewSQLMappingStore(ctx, db, clk.Now)
	_ = s.Put(ctx, "b", "1.1")
	clk.Advance(30 * 24 * time.Hour)
	purgeCheck(t, asPurger(t, s), 1)
	if got := countRows(t, db, "ga4_relay_client_map"); got != 0 {
		t.Fatalf("rows=%d want 0", got)
	}
}

// Scenario: Mapping expires - memory store.
func TestPurge_MappingStore_Memory_DeletesExpired(t *testing.T) {
	ctx := context.Background()
	clk := &fakeClock{t: time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)}
	s := NewMemoryMappingStore(clk.Now)
	p := asPurger(t, s)
	_ = s.Put(ctx, "old", "1.1")
	clk.Advance(20 * 24 * time.Hour)
	_ = s.Put(ctx, "fresh", "2.2")
	clk.Advance(11 * 24 * time.Hour)
	purgeCheck(t, p, 1)
	if _, ok := mustGet(t.Context(), t, s, "fresh"); !ok {
		t.Fatal("fresh mapping lost")
	}
	purgeCheck(t, p, 0)
}

// Scenario: duplicate delivery dedupe ledger (90 d retention) - expired rows physically removed.
func TestPurge_Ledger_DuckDB_DeletesExpiredRowsKeepsFresh(t *testing.T) {
	ctx := context.Background()
	clk := &fakeClock{t: time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)}
	db := openDB(t)
	l, err := NewSQLLedger(ctx, db, clk.Now)
	if err != nil {
		t.Fatal(err)
	}
	p := asPurger(t, l)
	_ = l.MarkSent(ctx, "o-old1")
	_ = l.MarkSent(ctx, "o-old2")
	clk.Advance(60 * 24 * time.Hour)
	_ = l.MarkSent(ctx, "o-fresh")
	purgeCheck(t, p, 0)
	clk.Advance(31 * 24 * time.Hour) // old* at 91d, fresh at 31d
	purgeCheck(t, p, 2)
	if got := countRows(t, db, "ga4_relay_sent"); got != 1 {
		t.Fatalf("rows after purge=%d want 1", got)
	}
	if sent, err := l.Sent(ctx, "o-fresh"); err != nil || !sent {
		t.Fatalf("fresh entry lost: %v,%v", sent, err)
	}
	purgeCheck(t, p, 0)
}

// Scenario: ledger retention - memory ledger must not grow unbounded.
func TestPurge_Ledger_Memory_DeletesExpired(t *testing.T) {
	ctx := context.Background()
	clk := &fakeClock{t: time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)}
	l := NewMemoryLedger(clk.Now)
	p := asPurger(t, l)
	_ = l.MarkSent(ctx, "o-old")
	clk.Advance(60 * 24 * time.Hour)
	_ = l.MarkSent(ctx, "o-fresh")
	clk.Advance(31 * 24 * time.Hour)
	purgeCheck(t, p, 1)
	if sent, _ := l.Sent(ctx, "o-fresh"); !sent {
		t.Fatal("fresh entry lost")
	}
	purgeCheck(t, p, 0)
}
