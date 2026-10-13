package ga4relay

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/buidangphuc/team-analytics/internal/store/duckdb"
)

// ledgerContract runs the same Ledger behaviour against every implementation.
func ledgerContract(t *testing.T, mk func(t *testing.T, clk *fakeClock) Ledger) {
	ctx := context.Background()
	t.Run("unknown order is not sent", func(t *testing.T) {
		l := mk(t, &fakeClock{t: time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)})
		if ok, err := l.Sent(ctx, "o1"); err != nil || ok {
			t.Fatalf("Sent = %v,%v; want false,nil", ok, err)
		}
	})
	t.Run("marked order is sent, others are not", func(t *testing.T) {
		l := mk(t, &fakeClock{t: time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)})
		if err := l.MarkSent(ctx, "o1"); err != nil {
			t.Fatal(err)
		}
		if ok, _ := l.Sent(ctx, "o1"); !ok {
			t.Fatal("o1 not sent")
		}
		if ok, _ := l.Sent(ctx, "o2"); ok {
			t.Fatal("o2 wrongly sent")
		}
	})
	t.Run("MarkSent is idempotent", func(t *testing.T) {
		l := mk(t, &fakeClock{t: time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)})
		if err := l.MarkSent(ctx, "o1"); err != nil {
			t.Fatal(err)
		}
		if err := l.MarkSent(ctx, "o1"); err != nil {
			t.Fatalf("second MarkSent: %v", err)
		}
	})
	t.Run("retained 90 days", func(t *testing.T) {
		clk := &fakeClock{t: time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)}
		l := mk(t, clk)
		_ = l.MarkSent(ctx, "o1")
		clk.Advance(89 * 24 * time.Hour)
		if ok, _ := l.Sent(ctx, "o1"); !ok {
			t.Fatal("entry gone before 90 days")
		}
		clk.Advance(2 * 24 * time.Hour)
		if ok, _ := l.Sent(ctx, "o1"); ok {
			t.Fatal("entry still present after 90 days")
		}
	})
}

func TestLedgerContract_Memory(t *testing.T) { // Scenarios: Kafka redelivery / Restart after send
	ledgerContract(t, func(_ *testing.T, clk *fakeClock) Ledger { return NewMemoryLedger(clk.Now) })
}

func TestLedgerContract_DuckDB(t *testing.T) { // Scenario: Restart after send (durable storage)
	ledgerContract(t, func(t *testing.T, clk *fakeClock) Ledger {
		ctx := context.Background()
		w, err := duckdb.Open(ctx, filepath.Join(t.TempDir(), "wh.duckdb"))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = w.Close() })
		l, err := NewSQLLedger(ctx, w.DB(), clk.Now)
		if err != nil {
			t.Fatal(err)
		}
		return l
	})
}
