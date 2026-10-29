package ga4import_test

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

	sdkconfig "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/mockdata"

	"github.com/buidangphuc/team-analytics/internal/config"
	"github.com/buidangphuc/team-analytics/internal/ga4import"
	"github.com/buidangphuc/team-analytics/internal/warehouse/duckdb"
)

func mockSet(t *testing.T, adapters string) *mockdata.Set {
	t.Helper()
	s, err := mockdata.New(sdkconfig.MockData{Adapters: adapters}, "ga4export")
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func newImporter(t *testing.T, cfg config.GA4, adapters string, w *duckdb.Writer) *ga4import.Importer {
	t.Helper()
	return ga4import.NewImporter(cfg, mockSet(t, adapters), w)
}

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

func sessionCount(t *testing.T, w *duckdb.Writer) int {
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

func TestMockDataGA4ExportRunsRealImporterOnFixtures(t *testing.T) {
	d := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	writeFixture(t, dir, d, []ev{
		{"u1", "google", "cpc", "c1", "https://agora.test/p/1", 101, at(d, 10, 0)},
		{"u1", "newsletter", "email", "c2", "https://agora.test/p/1", 102, at(d, 14, 0)},
		{"u2", "(direct)", "(none)", "(direct)", "https://agora.test/p/1", 201, at(d, 9, 0)},
	})
	w := open(t)
	h := ga4import.TriggerHandler(newImporter(t, config.GA4{FixtureDir: dir}, "ga4export", w))
	rec := post(h, "?date=2026-01-01")
	if rec.Code != 200 {
		t.Fatalf("status %d body %q", rec.Code, rec.Body.String())
	}
	if n := len(sessions(t, w, "2026-01-01")); n != 3 {
		t.Errorf("ga4_sessions rows for the date = %d, want 3", n)
	}
	var runs int
	if err := w.DB().QueryRow("SELECT count(*) FROM ga4_import_runs WHERE CAST(export_date AS VARCHAR)='2026-01-01'").Scan(&runs); err != nil || runs != 1 {
		t.Errorf("import runs = %d, err %v, want 1", runs, err)
	}
}

func TestMockDataGA4ExportRunsRealImporterOnFixtures_FixtureWinsOverADataset(t *testing.T) {
	dir := t.TempDir()
	writeEvents(t, dir)
	w := open(t)
	imp := newImporter(t, config.GA4{ExportDataset: "proj.never_read", FixtureDir: dir}, "ga4export", w)
	rows, err := imp.ImportDay(context.Background(), time.Date(2024, 3, 5, 0, 0, 0, 0, time.UTC))
	if err != nil || rows != 4 {
		t.Fatalf("rows=%d err=%v, want 4,nil", rows, err)
	}
	if n := sessionCount(t, w); n != 2 {
		t.Errorf("ga4_sessions = %d, want 2", n)
	}
}

func TestMockDataWithoutFixtureDirFailsNamingTheSetting(t *testing.T) {
	w := open(t)
	h := ga4import.TriggerHandler(newImporter(t, config.GA4{}, "ga4export", w))
	rec := post(h, "?date=2026-01-01")
	if rec.Code < 500 || rec.Code > 599 {
		t.Fatalf("status %d, want 5xx", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "GA4_EXPORT_FIXTURE_DIR") {
		t.Errorf("body must name GA4_EXPORT_FIXTURE_DIR, got %q", rec.Body.String())
	}
	if n := sessionCount(t, w); n != 0 {
		t.Errorf("ga4_sessions = %d, want 0", n)
	}
}

func TestFixtureDirIgnoredWithoutMockData(t *testing.T) {
	dir := t.TempDir()
	writeEvents(t, dir)
	w := open(t)
	h := ga4import.TriggerHandler(newImporter(t, config.GA4{FixtureDir: dir}, "", w))
	rec := post(h, "?date=2024-03-05")
	if rec.Code != 200 {
		t.Fatalf("status %d body %q", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"rows":0`) {
		t.Errorf("body must report rows 0, got %q", rec.Body.String())
	}
	if n := sessionCount(t, w); n != 0 {
		t.Errorf("ga4_sessions = %d, want unchanged 0", n)
	}
}

func TestTriggerImportsFixtureDateThroughTheConsumerWriter_TheImportRunsInsideTheConsumerWithoutASecondWriter(t *testing.T) {
	dir := t.TempDir()
	writeEvents(t, dir)
	w := open(t)
	h := ga4import.TriggerHandler(newImporter(t, config.GA4{FixtureDir: dir}, "ga4export", w))
	rec := post(h, "?date=2024-03-05")
	if rec.Code != 200 {
		t.Fatalf("status %d body %q", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "4") {
		t.Errorf("body must carry the row count 4, got %q", rec.Body.String())
	}
	if n := sessionCount(t, w); n != 2 {
		t.Errorf("ga4_sessions = %d via the consumer writer, want 2", n)
	}
	if rec := post(h, "?date=2024-03-05"); rec.Code != 200 {
		t.Errorf("re-trigger status %d", rec.Code)
	}
	if n := sessionCount(t, w); n != 2 {
		t.Errorf("after re-trigger ga4_sessions = %d, want 2", n)
	}
}

func TestTriggerRejectsInvalidDateWith4xx_AnInvalidDateIsRejectedByTheTrigger(t *testing.T) {
	dir := t.TempDir()
	writeEvents(t, dir)
	w := open(t)
	h := ga4import.TriggerHandler(newImporter(t, config.GA4{FixtureDir: dir}, "ga4export", w))
	for _, q := range []string{"?date=not-a-date", "?date=2024-13-40", "", "?date="} {
		rec := post(h, q)
		if rec.Code < 400 || rec.Code > 499 {
			t.Errorf("query %q: status %d, want 4xx", q, rec.Code)
		}
	}
	if n := sessionCount(t, w); n != 0 {
		t.Errorf("ga4_sessions = %d, want unchanged 0", n)
	}
}

func TestTriggerRejectsNonPost_AnInvalidDateIsRejectedByTheTrigger(t *testing.T) {
	w := open(t)
	h := ga4import.TriggerHandler(newImporter(t, config.GA4{FixtureDir: t.TempDir()}, "ga4export", w))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/internal/ga4import?date=2024-03-05", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET status %d, want 405", rec.Code)
	}
}

func TestTriggerReturns5xxWithDateWhenExportTableMissing_AFailedImportReturns5xxWithTheDate(t *testing.T) {
	dir := t.TempDir()
	writeEvents(t, dir)
	w := open(t)
	h := ga4import.TriggerHandler(newImporter(t, config.GA4{FixtureDir: dir}, "ga4export", w))
	rec := post(h, "?date=2024-03-06")
	if rec.Code < 500 || rec.Code > 599 {
		t.Fatalf("status %d, want 5xx", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "2024-03-06") {
		t.Errorf("body must name the date, got %q", rec.Body.String())
	}
	if n := sessionCount(t, w); n != 0 {
		t.Errorf("ga4_sessions = %d after failed import, want 0", n)
	}
}

func TestTriggerIsANoOpWhenUnconfigured_TheJobIsANoOpWhenUnconfigured(t *testing.T) {
	w := open(t)
	h := ga4import.TriggerHandler(newImporter(t, config.GA4{ReconcileMaxDiffRatio: 0.35}, "", w))
	rec := post(h, "?date=2024-03-05")
	if rec.Code != 200 {
		t.Fatalf("status %d body %q", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"rows":0`) {
		t.Errorf("body must report rows 0, got %q", rec.Body.String())
	}
	if n := sessionCount(t, w); n != 0 {
		t.Errorf("wrote %d rows, want 0", n)
	}
}

func TestScheduledRunIsANoOpWhenUnconfigured_TheJobIsANoOpWhenUnconfigured(t *testing.T) {
	w := open(t)
	imp := newImporter(t, config.GA4{ReconcileMaxDiffRatio: 0.35}, "", w)
	day := ga4import.ImportDate(time.Date(2024, 3, 6, 3, 30, 0, 0, time.UTC))
	rows, err := imp.ImportDay(context.Background(), day)
	if err != nil || rows != 0 {
		t.Fatalf("rows=%d err=%v, want 0,nil", rows, err)
	}
	if n := sessionCount(t, w); n != 0 {
		t.Errorf("wrote %d rows, want 0", n)
	}
}

func TestSchedulerPicksPreviousUTCDateAndNextRun_TheImportRunsInsideTheConsumerWithoutASecondWriter(t *testing.T) {
	now := time.Date(2024, 3, 6, 3, 30, 0, 0, time.UTC)
	if got, want := ga4import.ImportDate(now), time.Date(2024, 3, 5, 0, 0, 0, 0, time.UTC); !got.Equal(want) {
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
			t.Errorf("NextRun(%v,%q) = %v,%v want %v", c.now, c.at, got, err, c.want)
		}
	}
	for _, bad := range []string{"", "25:00", "3pm", "03:99"} {
		if _, err := ga4import.NextRun(now, bad); err == nil {
			t.Errorf("NextRun(%q) must fail", bad)
		}
	}
}
