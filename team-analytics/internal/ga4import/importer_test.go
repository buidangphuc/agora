package ga4import_test

// Seams the implementer must provide in package ga4import:
//   NewImporter(cfg config.GA4, w *duckdb.Writer) *Importer
//   (*Importer).ImportDay(ctx context.Context, day time.Time) (rows int64, err error)
//       - unconfigured (ExportDataset=="" and FixtureDir=="") => 0, nil, writes nothing
//       - uses ONLY the passed writer (no duckdb.Open)
//   TriggerHandler(imp *Importer) http.Handler   // POST /internal/ga4import?date=YYYY-MM-DD
//   NextRun(now time.Time, at string) (time.Time, error)  // next UTC "HH:MM" after now
//   PreviousDay(now time.Time) time.Time                    // previous UTC date (00:00 UTC)

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "github.com/marcboeker/go-duckdb"

	"github.com/buidangphuc/team-analytics/internal/config"
	"github.com/buidangphuc/team-analytics/internal/ga4import"
	"github.com/buidangphuc/team-analytics/internal/store/duckdb"
)

func writeEvents(t *testing.T, dir string) {
	t.Helper()
	db, err := sql.Open("duckdb", "")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec(fmt.Sprintf(`COPY (SELECT '20240305' AS event_date, CAST(1709632800000000 + i * 1000000 AS BIGINT) AS event_timestamp,
	 'view_item' AS event_name, 'u' || CAST(i %% 2 AS VARCHAR) AS user_pseudo_id,
	 [ {'key':'ga_session_id','value':{'string_value':CAST(NULL AS VARCHAR),'int_value':CAST(i %% 2 + 1 AS BIGINT)}},
	   {'key':'source','value':{'string_value':'google','int_value':CAST(NULL AS BIGINT)}},
	   {'key':'medium','value':{'string_value':'cpc','int_value':CAST(NULL AS BIGINT)}},
	   {'key':'campaign','value':{'string_value':'c','int_value':CAST(NULL AS BIGINT)}},
	   {'key':'page_location','value':{'string_value':'https://agora.test/p/1?x=1','int_value':CAST(NULL AS BIGINT)}} ] AS event_params,
	 {'name':'c','medium':'cpc','source':'google'} AS traffic_source FROM range(4) t(i)) TO '%s' (FORMAT PARQUET)`,
		filepath.Join(dir, "events_20240305.parquet")))
	if err != nil {
		t.Fatal(err)
	}
}

func openWriter(t *testing.T) *duckdb.Writer {
	t.Helper()
	w, err := duckdb.Open(context.Background(), filepath.Join(t.TempDir(), "w.duckdb"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = w.Close() })
	return w
}

func writerSessions(t *testing.T, w *duckdb.Writer) int {
	t.Helper()
	var n int
	if err := w.DB().QueryRow("SELECT count(*) FROM ga4_sessions").Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func post(h http.Handler, query string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/internal/ga4import"+query, nil))
	return rec
}

func fixtureCfg(dir string) config.GA4 {
	return config.GA4{ExportSource: "fixture", ExportDataset: "fixture", FixtureDir: dir, ReconcileMaxDiffRatio: 0.35}
}

// Scenario: The import runs inside the consumer without a second writer
func TestTriggerImportsFixtureDateThroughTheConsumerWriter_TheImportRunsInsideTheConsumerWithoutASecondWriter(t *testing.T) {
	dir := t.TempDir()
	writeEvents(t, dir)
	w := openWriter(t)
	h := ga4import.TriggerHandler(ga4import.NewImporter(fixtureCfg(dir), w))
	rec := post(h, "?date=2024-03-05")
	if rec.Code != 200 {
		t.Fatalf("status %d body %q", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "4") {
		t.Errorf("body must carry the row count 4, got %q", rec.Body.String())
	}
	if n := writerSessions(t, w); n != 2 {
		t.Errorf("ga4_sessions = %d via the consumer writer, want 2", n)
	}
	// Idempotent re-trigger on the same writer still works (no second connection/lock needed).
	if rec := post(h, "?date=2024-03-05"); rec.Code != 200 {
		t.Errorf("re-trigger status %d", rec.Code)
	}
	if n := writerSessions(t, w); n != 2 {
		t.Errorf("after re-trigger ga4_sessions = %d, want 2", n)
	}
}

// Scenario: An invalid date is rejected by the trigger
func TestTriggerRejectsInvalidDateWith4xx_AnInvalidDateIsRejectedByTheTrigger(t *testing.T) {
	dir := t.TempDir()
	writeEvents(t, dir)
	w := openWriter(t)
	h := ga4import.TriggerHandler(ga4import.NewImporter(fixtureCfg(dir), w))
	for _, q := range []string{"?date=not-a-date", "?date=2024-13-40", "", "?date="} {
		rec := post(h, q)
		if rec.Code < 400 || rec.Code > 499 {
			t.Errorf("query %q: status %d, want 4xx", q, rec.Code)
		}
	}
	if n := writerSessions(t, w); n != 0 {
		t.Errorf("ga4_sessions = %d, want unchanged 0", n)
	}
}

// Scenario: An invalid date is rejected by the trigger (only POST is accepted)
func TestTriggerRejectsNonPost_AnInvalidDateIsRejectedByTheTrigger(t *testing.T) {
	w := openWriter(t)
	h := ga4import.TriggerHandler(ga4import.NewImporter(fixtureCfg(t.TempDir()), w))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/internal/ga4import?date=2024-03-05", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET status %d, want 405", rec.Code)
	}
}

// Scenario: A failed import returns 5xx with the date
func TestTriggerReturns5xxWithDateWhenExportTableMissing_AFailedImportReturns5xxWithTheDate(t *testing.T) {
	dir := t.TempDir()
	writeEvents(t, dir)
	w := openWriter(t)
	h := ga4import.TriggerHandler(ga4import.NewImporter(fixtureCfg(dir), w))
	rec := post(h, "?date=2024-03-06")
	if rec.Code < 500 || rec.Code > 599 {
		t.Fatalf("status %d, want 5xx", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "2024-03-06") {
		t.Errorf("body must name the date, got %q", rec.Body.String())
	}
	if n := writerSessions(t, w); n != 0 {
		t.Errorf("ga4_sessions = %d after failed import, want 0", n)
	}
}

// Scenario: The job is a no-op when unconfigured (trigger)
func TestTriggerIsANoOpWhenUnconfigured_TheJobIsANoOpWhenUnconfigured(t *testing.T) {
	w := openWriter(t)
	h := ga4import.TriggerHandler(ga4import.NewImporter(config.GA4{ExportSource: "bigquery", ReconcileMaxDiffRatio: 0.35}, w))
	rec := post(h, "?date=2024-03-05")
	if rec.Code != 200 {
		t.Fatalf("status %d body %q", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "0") {
		t.Errorf("body must report rows 0, got %q", rec.Body.String())
	}
	if n := writerSessions(t, w); n != 0 {
		t.Errorf("wrote %d rows, want 0", n)
	}
}

// Scenario: The job is a no-op when unconfigured (scheduled run)
func TestScheduledRunIsANoOpWhenUnconfigured_TheJobIsANoOpWhenUnconfigured(t *testing.T) {
	w := openWriter(t)
	imp := ga4import.NewImporter(config.GA4{ExportSource: "bigquery", ReconcileMaxDiffRatio: 0.35}, w)
	day := ga4import.PreviousDay(time.Date(2024, 3, 6, 3, 30, 0, 0, time.UTC))
	rows, err := imp.ImportDay(context.Background(), day)
	if err != nil || rows != 0 {
		t.Fatalf("rows=%d err=%v, want 0,nil", rows, err)
	}
	if n := writerSessions(t, w); n != 0 {
		t.Errorf("wrote %d rows, want 0", n)
	}
}

// Scenario: The import runs inside the consumer without a second writer (daily scheduler timing)
func TestSchedulerPicksPreviousUTCDateAndNextRun_TheImportRunsInsideTheConsumerWithoutASecondWriter(t *testing.T) {
	now := time.Date(2024, 3, 6, 3, 30, 0, 0, time.UTC)
	if got, want := ga4import.PreviousDay(now), time.Date(2024, 3, 5, 0, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Errorf("import day = %v, want %v", got, want)
	}
	cases := []struct {
		now  time.Time
		at   string
		want time.Time
	}{
		{time.Date(2024, 3, 5, 2, 0, 0, 0, time.UTC), "03:30", time.Date(2024, 3, 5, 3, 30, 0, 0, time.UTC)},
		{time.Date(2024, 3, 5, 10, 0, 0, 0, time.UTC), "03:30", time.Date(2024, 3, 6, 3, 30, 0, 0, time.UTC)},
		{time.Date(2024, 3, 5, 3, 30, 0, 0, time.UTC), "03:30", time.Date(2024, 3, 6, 3, 30, 0, 0, time.UTC)},
	}
	for _, c := range cases {
		got, err := ga4import.NextRun(c.now, c.at)
		if err != nil || !got.Equal(c.want) {
			t.Errorf("ga4import.NextRun(%v,%q) = %v,%v want %v", c.now, c.at, got, err, c.want)
		}
	}
	for _, bad := range []string{"", "25:00", "3pm", "03:99"} {
		if _, err := ga4import.NextRun(now, bad); err == nil {
			t.Errorf("ga4import.NextRun(%q) must fail", bad)
		}
	}
}
