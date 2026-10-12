package export_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/buidangphuc/team-analytics/internal/export"
	"github.com/buidangphuc/team-analytics/internal/warehouse"
	"github.com/buidangphuc/team-analytics/internal/warehouse/duckdb"
)

// fileExporter writes body to dst, or fails with err.
type fileExporter struct {
	body    string
	err     error
	calls   atomic.Int32
	seen    []string
	failRel string
}

func (f *fileExporter) ExportRelation(_ context.Context, rel, dst string) error {
	f.calls.Add(1)
	f.seen = append(f.seen, dst)
	if f.err != nil || f.failRel == rel {
		if f.err == nil {
			_ = os.WriteFile(dst, []byte("partial"), 0o600)
			return errors.New("fail " + rel)
		}
		_ = os.WriteFile(dst, []byte("partial"), 0o600)
		return f.err
	}
	return os.WriteFile(dst, []byte(f.body+rel), 0o600)
}

func TestOnce_ReplacesAtomicallyViaTempFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tracking_events.parquet")
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	e := &fileExporter{body: "new"}
	if err := export.Once(context.Background(), e, "r", path); err != nil {
		t.Fatalf("Once: %v", err)
	}
	if e.seen[0] != path+".tmp" {
		t.Errorf("exported to %q, want the temp file %q", e.seen[0], path+".tmp")
	}
	got, _ := os.ReadFile(path)
	if string(got) != "newr" {
		t.Errorf("target = %q, want newr", got)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Errorf("temp file left behind: %v", err)
	}
}

func TestOnce_FailureKeepsThePreviousFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tracking_events.parquet")
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	e := &fileExporter{err: errors.New("disk full")}
	if err := export.Once(context.Background(), e, "r", path); err == nil {
		t.Fatal("want an error")
	}
	got, _ := os.ReadFile(path)
	if string(got) != "old" {
		t.Errorf("target = %q, want the previous export kept", got)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Errorf("partial temp file left behind: %v", err)
	}
}

func TestEnabled_OffByDefault(t *testing.T) {
	for _, c := range []struct {
		path     string
		interval int
		want     bool
	}{{"", 0, false}, {"/data/x.parquet", 0, false}, {"", 60, false}, {"/data/x.parquet", 60, true}} {
		if got := export.Enabled(c.path, c.interval); got != c.want {
			t.Errorf("Enabled(%q, %d) = %v, want %v", c.path, c.interval, got, c.want)
		}
	}
}

func TestRun_FailuresDoNotStopTheLoop(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tracking_events.parquet")
	e := &fileExporter{err: errors.New("boom")}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		export.Run(ctx, e, path, 5*time.Millisecond, slog.New(slog.NewTextHandler(io.Discard, nil)))
		close(done)
	}()
	deadline := time.Now().Add(2 * time.Second)
	for e.calls.Load() < 3 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	<-done
	if n := e.calls.Load(); n < 3 {
		t.Fatalf("export ran %d times; a failure must not stop the loop", n)
	}
}

func TestCycle_WritesEveryFileBesideTheTrackingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tracking_events.parquet")
	e := &fileExporter{body: "x-"}
	if err := export.Cycle(context.Background(), e, path, slog.New(slog.NewTextHandler(io.Discard, nil))); err != nil {
		t.Fatalf("Cycle: %v", err)
	}
	for _, name := range []string{"tracking_events", "tracking_events_resolved", "engagement_facts", "order_facts", "listing_sellers"} {
		got, err := os.ReadFile(filepath.Join(dir, name+".parquet"))
		if err != nil || string(got) != "x-"+name {
			t.Errorf("%s.parquet = %q, %v", name, got, err)
		}
	}
	if left, _ := filepath.Glob(filepath.Join(dir, "*.tmp")); len(left) != 0 {
		t.Errorf("temp files left behind: %v", left)
	}
}

func TestCycle_OneFailureDoesNotBlockTheOthers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tracking_events.parquet")
	old := filepath.Join(dir, "engagement_facts.parquet")
	if err := os.WriteFile(old, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	e := &fileExporter{body: "x-", failRel: "engagement_facts"}
	if err := export.Cycle(context.Background(), e, path, slog.New(slog.NewTextHandler(io.Discard, nil))); err == nil {
		t.Fatal("want the failure reported")
	}
	for _, name := range []string{"tracking_events", "tracking_events_resolved", "order_facts"} {
		if _, err := os.Stat(filepath.Join(dir, name+".parquet")); err != nil {
			t.Errorf("%s.parquet missing after a sibling failed: %v", name, err)
		}
	}
	if got, _ := os.ReadFile(old); string(got) != "old" {
		t.Errorf("failed file = %q, want the previous export kept", got)
	}
	if left, _ := filepath.Glob(filepath.Join(dir, "*.tmp")); len(left) != 0 {
		t.Errorf("temp files left behind: %v", left)
	}
}

func openWriter(t *testing.T) *duckdb.Writer {
	t.Helper()
	w, err := duckdb.Open(context.Background(), filepath.Join(t.TempDir(), "wh.duckdb"))
	if err != nil {
		t.Fatalf("open duckdb: %v", err)
	}
	t.Cleanup(func() { _ = w.Close() })
	return w
}

// Scenario: Exported file has ecommerce columns
func TestExport_ExportedFileHasEcommerceColumns(t *testing.T) {
	ctx := context.Background()
	w := openWriter(t)
	rec := &warehouse.TrackingRecord{
		EventID: "e-1", EventType: "purchase", ListingID: "l-1", SessionID: "s-1",
		OccurredAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		Currency:   "VND", Value: 500000, TransactionID: "o-1",
		ItemListID: "home_feed", EventGroupID: "g-1", Position: 3,
	}
	if err := w.Write(ctx, []*warehouse.TrackingRecord{rec}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	path := filepath.Join(t.TempDir(), "tracking_events.parquet")
	if err := export.Once(ctx, w, warehouse.TableName, path); err != nil {
		t.Fatalf("Once: %v", err)
	}

	cols := []string{"item_list_id", "position", "event_group_id", "transaction_id", "currency", "value"}
	for _, c := range cols {
		var n int
		err := w.DB().QueryRowContext(ctx,
			"SELECT count(*) FROM parquet_schema('"+path+"') WHERE name = ?", c).Scan(&n)
		if err != nil || n != 1 {
			t.Errorf("parquet column %q present=%d err=%v", c, n, err)
		}
	}

	var listID, group, txn, cur string
	var pos, val int64
	err := w.DB().QueryRowContext(ctx,
		"SELECT item_list_id, position, event_group_id, transaction_id, currency, value FROM read_parquet('"+path+"') WHERE event_id = 'e-1'").
		Scan(&listID, &pos, &group, &txn, &cur, &val)
	if err != nil {
		t.Fatalf("read back parquet: %v", err)
	}
	if listID != "home_feed" || pos != 3 || group != "g-1" || txn != "o-1" || cur != "VND" || val != 500000 {
		t.Errorf("row = %q %d %q %q %q %d", listID, pos, group, txn, cur, val)
	}
}

// Scenario: Failed export keeps previous file
func TestExport_FailedExportKeepsPreviousFile(t *testing.T) {
	ctx := context.Background()
	w := openWriter(t)
	if err := w.Write(ctx, []*warehouse.TrackingRecord{{
		EventID: "e-1", EventType: "purchase", OccurredAt: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
		TransactionID: "o-1", Currency: "VND", Value: 500000,
	}}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "tracking_events.parquet")
	if err := export.Once(ctx, w, warehouse.TableName, path); err != nil {
		t.Fatalf("first export: %v", err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	// The export query fails: the relation does not exist.
	if err := export.Once(ctx, w, "no_such_relation", path); err == nil {
		t.Fatal("want the export query to fail")
	}

	after, err := os.ReadFile(path)
	if err != nil || string(after) != string(before) {
		t.Errorf("previous parquet changed after failed export (err=%v)", err)
	}
	var n int
	if err := w.DB().QueryRowContext(ctx, "SELECT count(*) FROM read_parquet('"+path+"') WHERE transaction_id = 'o-1'").Scan(&n); err != nil || n != 1 {
		t.Errorf("previous file unreadable or changed: n=%d err=%v", n, err)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Errorf("temp file left behind: %v", err)
	}
}
