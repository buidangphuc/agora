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

// Scenario: Import of a fixture day produces sessions (export half: the cycle ships the GA4 relations)
func TestCycle_ImportOfAFixtureDayProducesSessions_WritesGA4Files(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tracking_events.parquet")
	e := &fileExporter{body: "x-"}
	if err := export.Cycle(context.Background(), e, path, slog.New(slog.NewTextHandler(io.Discard, nil))); err != nil {
		t.Fatalf("Cycle: %v", err)
	}
	for _, name := range []string{"ga4_sessions", "tracking_events_attributed"} {
		got, err := os.ReadFile(filepath.Join(dir, name+".parquet"))
		if err != nil || string(got) != "x-"+name {
			t.Errorf("%s.parquet = %q, %v (relation must be exported under its own name)", name, got, err)
		}
	}
}

// Scenario: Import of a fixture day produces sessions (a GA4 export failure keeps the previous file and blocks nothing)
func TestCycle_ImportOfAFixtureDayProducesSessions_GA4FailureIsIsolated(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tracking_events.parquet")
	old := filepath.Join(dir, "ga4_sessions.parquet")
	if err := os.WriteFile(old, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	e := &fileExporter{body: "x-", failRel: "ga4_sessions"}
	if err := export.Cycle(context.Background(), e, path, slog.New(slog.NewTextHandler(io.Discard, nil))); err == nil {
		t.Fatal("want the failure reported")
	}
	if got, _ := os.ReadFile(old); string(got) != "old" {
		t.Errorf("ga4_sessions.parquet = %q, want the previous export kept", got)
	}
	for _, name := range []string{"tracking_events", "tracking_events_attributed"} {
		if _, err := os.Stat(filepath.Join(dir, name+".parquet")); err != nil {
			t.Errorf("%s.parquet missing after ga4_sessions failed: %v", name, err)
		}
	}
}

// Scenario: Import of a fixture day produces sessions (real DuckDB: exported sessions and attributed view carry the data)
func TestExport_ImportOfAFixtureDayProducesSessions_RealRelations(t *testing.T) {
	ctx := context.Background()
	w := openWriter(t)
	if _, err := w.DB().ExecContext(ctx, `INSERT INTO ga4_sessions (export_date, user_pseudo_id, session_id, source, medium, campaign, landing_page, ts, ended_at) VALUES
	 (DATE '2026-10-01', '111.222', 's1', 'google', 'cpc', 'spring', 'https://agora.test/p/1', TIMESTAMP '2026-10-01 10:00:00', TIMESTAMP '2026-10-01 10:15:00'),
	 (DATE '2026-10-01', '111.222', 's2', 'direct', '(none)', '', 'https://agora.test/', TIMESTAMP '2026-10-01 12:00:00', TIMESTAMP '2026-10-01 12:05:00'),
	 (DATE '2026-10-01', '333.444', 's3', 'bing', 'organic', '', 'https://agora.test/p/2', TIMESTAMP '2026-10-01 09:00:00', TIMESTAMP '2026-10-01 09:10:00')`); err != nil {
		t.Fatalf("seed ga4_sessions: %v", err)
	}
	if err := w.Write(ctx, []*warehouse.TrackingRecord{{
		EventID: "e-1", EventType: "view", ListingID: "l-1", SessionID: "x",
		OccurredAt: time.Date(2026, 10, 1, 10, 5, 0, 0, time.UTC), GaClientID: "111.222",
	}}); err != nil {
		t.Fatalf("Write: %v", err)
	}

	dir := t.TempDir()
	sessions := filepath.Join(dir, "ga4_sessions.parquet")
	attributed := filepath.Join(dir, "tracking_events_attributed.parquet")
	if err := export.Once(ctx, w, "ga4_sessions", sessions); err != nil {
		t.Fatalf("export ga4_sessions: %v", err)
	}
	if err := export.Once(ctx, w, "tracking_events_attributed", attributed); err != nil {
		t.Fatalf("export tracking_events_attributed: %v", err)
	}

	var n, nonEmpty int
	if err := w.DB().QueryRowContext(ctx, "SELECT count(*), count(*) FILTER (WHERE source <> '' AND medium <> '' AND session_id <> '') FROM read_parquet('"+sessions+"') WHERE export_date = DATE '2026-10-01'").Scan(&n, &nonEmpty); err != nil {
		t.Fatalf("read sessions parquet: %v", err)
	}
	if n != 3 || nonEmpty != 3 {
		t.Errorf("sessions parquet rows=%d with source/medium/session_id=%d, want 3 and 3", n, nonEmpty)
	}
	for _, c := range []string{"export_date", "ended_at", "user_pseudo_id", "landing_page"} {
		var k int
		if err := w.DB().QueryRowContext(ctx, "SELECT count(*) FROM parquet_schema('"+sessions+"') WHERE name = ?", c).Scan(&k); err != nil || k != 1 {
			t.Errorf("ga4_sessions parquet column %q present=%d err=%v", c, k, err)
		}
	}

	var src, med string
	var rows int
	if err := w.DB().QueryRowContext(ctx, "SELECT count(*) OVER (), source, medium FROM read_parquet('"+attributed+"') WHERE event_id = 'e-1'").Scan(&rows, &src, &med); err != nil {
		t.Fatalf("read attributed parquet: %v", err)
	}
	if rows != 1 || src != "google" || med != "cpc" {
		t.Errorf("attributed rows=%d source=%q medium=%q, want 1 google cpc", rows, src, med)
	}
}

const sessionSeed = `INSERT INTO ga4_sessions (export_date, user_pseudo_id, session_id, source, medium, campaign, landing_page, ts, ended_at) VALUES
 (DATE '2026-10-01', '111.222', 's1', 'google', 'cpc', 'spring', 'https://agora.test/p/1', TIMESTAMP '2026-10-01 10:00:00', TIMESTAMP '2026-10-01 10:15:00')`

var contextCols = []string{"consent_personalization", "consent_version", "item_list_id", "position", "session_source", "session_medium", "session_campaign"}

func parquetHasCols(t *testing.T, w *duckdb.Writer, path string, cols []string) {
	t.Helper()
	for _, c := range cols {
		var n int
		err := w.DB().QueryRowContext(context.Background(),
			"SELECT count(*) FROM parquet_schema('"+path+"') WHERE name = ?", c).Scan(&n)
		if err != nil || n != 1 {
			t.Errorf("parquet column %q present=%d err=%v", c, n, err)
		}
	}
}

// Scenario: Exported file has consent and context columns
func TestExport_ExportedFileHasConsentAndContextColumns(t *testing.T) {
	ctx := context.Background()
	w := openWriter(t)
	if _, err := w.DB().ExecContext(ctx, sessionSeed); err != nil {
		t.Fatalf("seed ga4_sessions: %v", err)
	}
	if err := w.Write(ctx, []*warehouse.TrackingRecord{{
		EventID: "e-1", EventType: "view", ListingID: "l-1", SessionID: "x",
		OccurredAt: time.Date(2026, 10, 1, 10, 5, 0, 0, time.UTC), GaClientID: "111.222",
		ConsentPersonalization: true, ConsentVersion: 2, ItemListID: "home_feed", Position: 4,
	}}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	path := filepath.Join(t.TempDir(), "tracking_events.parquet")
	if err := export.Once(ctx, w, warehouse.TableName, path); err != nil {
		t.Fatalf("Once: %v", err)
	}
	parquetHasCols(t, w, path, contextCols)

	var cp bool
	var cv, pos int64
	var list, src, med, camp string
	if err := w.DB().QueryRowContext(ctx, "SELECT consent_personalization, consent_version, item_list_id, position, session_source, session_medium, session_campaign FROM read_parquet('"+path+"') WHERE event_id = 'e-1'").
		Scan(&cp, &cv, &list, &pos, &src, &med, &camp); err != nil {
		t.Fatalf("read back parquet: %v", err)
	}
	if !cp || cv != 2 || list != "home_feed" || pos != 4 || src != "google" || med != "cpc" || camp != "spring" {
		t.Errorf("row = %v %d %q %d %q %q %q", cp, cv, list, pos, src, med, camp)
	}
}

// Scenario: Unmatched event has empty session context
func TestExport_UnmatchedEventHasEmptySessionContext(t *testing.T) {
	ctx := context.Background()
	w := openWriter(t)
	if _, err := w.DB().ExecContext(ctx, sessionSeed); err != nil {
		t.Fatalf("seed ga4_sessions: %v", err)
	}
	at := time.Date(2026, 10, 1, 10, 5, 0, 0, time.UTC)
	if err := w.Write(ctx, []*warehouse.TrackingRecord{
		{EventID: "e-empty", EventType: "view", ListingID: "l-1", SessionID: "x", OccurredAt: at, GaClientID: ""},
		{EventID: "e-nomatch", EventType: "view", ListingID: "l-2", SessionID: "y", OccurredAt: at, GaClientID: "999.000"},
	}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	path := filepath.Join(t.TempDir(), "tracking_events.parquet")
	if err := export.Once(ctx, w, warehouse.TableName, path); err != nil {
		t.Fatalf("Once: %v", err)
	}
	parquetHasCols(t, w, path, []string{"session_source", "session_medium", "session_campaign"})

	var rows, empty int
	if err := w.DB().QueryRowContext(ctx, "SELECT count(*), count(*) FILTER (WHERE session_source = '' AND session_medium = '' AND session_campaign = '') FROM read_parquet('"+path+"')").
		Scan(&rows, &empty); err != nil {
		t.Fatalf("read back parquet: %v", err)
	}
	if rows != 2 || empty != 2 {
		t.Errorf("rows=%d with empty session context=%d, want 2 and 2", rows, empty)
	}
}

// Scenario: Export never reads GA4 as an interaction source
func TestExport_NeverReadsGA4AsInteractionSource(t *testing.T) {
	ctx := context.Background()
	w := openWriter(t)
	if _, err := w.DB().ExecContext(ctx, sessionSeed); err != nil {
		t.Fatalf("seed ga4_sessions: %v", err)
	}
	path := filepath.Join(t.TempDir(), "tracking_events.parquet")
	if err := export.Once(ctx, w, warehouse.TableName, path); err != nil {
		t.Fatalf("Once: %v", err)
	}
	parquetHasCols(t, w, path, contextCols)
	var n int
	if err := w.DB().QueryRowContext(ctx, "SELECT count(*) FROM read_parquet('"+path+"')").Scan(&n); err != nil || n != 0 {
		t.Errorf("exported rows = %d (err=%v), want 0 when only ga4_sessions rows exist", n, err)
	}
}
