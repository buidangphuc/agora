package ga4import_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/buidangphuc/team-analytics/internal/ga4import"
	"github.com/buidangphuc/team-analytics/internal/warehouse/duckdb"
)

var (
	d1 = time.Date(2024, 3, 5, 0, 0, 0, 0, time.UTC)
	d2 = time.Date(2024, 3, 6, 0, 0, 0, 0, time.UTC)
)

func at(day time.Time, h, m int) time.Time { return day.Add(time.Duration(h)*time.Hour + time.Duration(m)*time.Minute) }

func open(t *testing.T) *duckdb.Writer {
	t.Helper()
	w, err := duckdb.Open(context.Background(), filepath.Join(t.TempDir(), "w.duckdb"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = w.Close() })
	return w
}

func run(t *testing.T, w *duckdb.Writer, dir string, day time.Time) (ga4import.Result, error) {
	t.Helper()
	return ga4import.Run(context.Background(), w, ga4import.NewFixtureSource(dir), day)
}

type sess struct {
	user, sid, source, medium, campaign, landing string
	ts, endedAt                                  time.Time
	exportDate                                   string
}

func sessions(t *testing.T, w *duckdb.Writer, day string) []sess {
	t.Helper()
	rows, err := w.DB().Query("SELECT user_pseudo_id, session_id, source, medium, campaign, landing_page, ts, ended_at, CAST(export_date AS VARCHAR) " +
		"FROM ga4_sessions WHERE CAST(export_date AS VARCHAR) = '" + day + "' ORDER BY user_pseudo_id, session_id")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []sess
	for rows.Next() {
		var s sess
		var ts, end sql.NullTime
		if err := rows.Scan(&s.user, &s.sid, &s.source, &s.medium, &s.campaign, &s.landing, &ts, &end, &s.exportDate); err != nil {
			t.Fatal(err)
		}
		s.ts, s.endedAt = ts.Time.UTC(), end.Time.UTC()
		out = append(out, s)
	}
	return out
}

func threeSessions(day time.Time) []ev {
	p := "https://agora.test/p/1"
	return []ev{
		{"u1", "google", "cpc", "spring", p, 101, at(day, 10, 0)},
		{"u1", "google", "cpc", "spring", p, 101, at(day, 10, 7)},
		{"u1", "google", "cpc", "spring", p, 101, at(day, 10, 15)},
		{"u1", "newsletter", "email", "summer", p, 102, at(day, 14, 0)},
		{"u2", "(direct)", "(none)", "(direct)", p, 201, at(day, 9, 0)},
	}
}

// Scenario: Import of a fixture day produces sessions
// Scenario: ga4_sessions stores ended_at
func TestImportOfAFixtureDayProducesSessions(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, d1, threeSessions(d1))
	w := open(t)
	if _, err := run(t, w, dir, d1); err != nil {
		t.Fatal(err)
	}
	got := sessions(t, w, "2024-03-05")
	if len(got) != 3 {
		t.Fatalf("want 3 sessions, got %d: %+v", len(got), got)
	}
	users := map[string]bool{}
	for _, s := range got {
		users[s.user] = true
		if s.source == "" || s.medium == "" || s.sid == "" {
			t.Errorf("empty source/medium/session_id: %+v", s)
		}
	}
	if len(users) != 2 {
		t.Errorf("want 2 distinct users, got %v", users)
	}
	// ended_at / ts of session u1/101: 10:00 .. 10:15
	for _, s := range got {
		if s.user == "u1" && s.sid == "101" {
			if !s.ts.Equal(at(d1, 10, 0)) || !s.endedAt.Equal(at(d1, 10, 15)) {
				t.Errorf("ts/ended_at = %v/%v, want 10:00/10:15", s.ts, s.endedAt)
			}
			if s.source != "google" || s.medium != "cpc" || s.campaign != "spring" {
				t.Errorf("channel = %s/%s/%s", s.source, s.medium, s.campaign)
			}
		}
	}
}

// Scenario: Re-running the same date is idempotent
// Scenario: ga4_sessions carries export_date (D2 untouched by D1 re-import)
func TestReRunningTheSameDateIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, d1, threeSessions(d1))
	writeFixture(t, dir, d2, []ev{{"u9", "bing", "organic", "x", "https://agora.test/p/9", 901, at(d2, 8, 0)}})
	w := open(t)
	for _, d := range []time.Time{d1, d2} {
		if _, err := run(t, w, dir, d); err != nil {
			t.Fatal(err)
		}
	}
	first := sessions(t, w, "2024-03-05")
	other := sessions(t, w, "2024-03-06")
	if _, err := run(t, w, dir, d1); err != nil {
		t.Fatal(err)
	}
	second := sessions(t, w, "2024-03-05")
	if len(first) != 3 || len(second) != 3 {
		t.Fatalf("counts %d then %d, want 3 and 3", len(first), len(second))
	}
	for i := range first {
		if first[i] != second[i] {
			t.Errorf("row %d changed: %+v vs %+v", i, first[i], second[i])
		}
		if first[i].exportDate == "" {
			t.Errorf("null export_date")
		}
	}
	after := sessions(t, w, "2024-03-06")
	if len(other) != 1 || len(after) != 1 || other[0] != after[0] {
		t.Errorf("D2 rows changed: %+v -> %+v", other, after)
	}
}

// Scenario: A missing export table fails without changing data
func TestMissingExportTableFailsWithoutChangingData(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, d1, threeSessions(d1))
	w := open(t)
	if _, err := run(t, w, dir, d1); err != nil {
		t.Fatal(err)
	}
	before := sessions(t, w, "2024-03-05")
	_, err := run(t, w, dir, d2) // no events_20240306.parquet
	if err == nil {
		t.Fatal("want error for missing export table")
	}
	if !strings.Contains(err.Error(), "2024-03-06") && !strings.Contains(err.Error(), "20240306") {
		t.Errorf("error must name the date, got %v", err)
	}
	if got := sessions(t, w, "2024-03-05"); len(got) != len(before) {
		t.Errorf("other-date rows changed: %d -> %d", len(before), len(got))
	}
	if n := sessions(t, w, "2024-03-06"); len(n) != 0 {
		t.Errorf("rows written for failed date: %+v", n)
	}
	var runs int
	if err := w.DB().QueryRow("SELECT count(*) FROM ga4_import_runs WHERE CAST(export_date AS VARCHAR)='2024-03-06'").Scan(&runs); err != nil || runs != 0 {
		t.Errorf("import run recorded for failed date: %d, %v", runs, err)
	}
}

// Scenario: A missing export table fails without changing data (a failed re-import keeps the old rows of the same date)
func TestMissingTableForReimportedDateKeepsExistingRows(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, d1, threeSessions(d1))
	w := open(t)
	if _, err := run(t, w, dir, d1); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, w, t.TempDir(), d1); err == nil { // empty dir: table gone
		t.Fatal("want error")
	}
	if got := sessions(t, w, "2024-03-05"); len(got) != 3 {
		t.Errorf("rows of the date were modified by a failed run: %d", len(got))
	}
}

// Scenario: Landing page query strings are removed
func TestLandingPageQueryStringsAreRemoved(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, d1, []ev{{"u1", "google", "cpc", "c", "https://agora.test/p/1?email=a@b.com&utm_source=x", 7, at(d1, 10, 0)}})
	w := open(t)
	if _, err := run(t, w, dir, d1); err != nil {
		t.Fatal(err)
	}
	got := sessions(t, w, "2024-03-05")
	if len(got) != 1 || got[0].landing != "https://agora.test/p/1" {
		t.Fatalf("landing_page = %+v, want https://agora.test/p/1", got)
	}
}

// Scenario: Empty day is a valid import
func TestEmptyDayIsAValidImport(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, d1, nil)
	w := open(t)
	res, err := run(t, w, dir, d1)
	if err != nil {
		t.Fatal(err)
	}
	if res.Rows != 0 || len(sessions(t, w, "2024-03-05")) != 0 {
		t.Errorf("rows = %d, want 0", res.Rows)
	}
	var n int64 = -1
	if err := w.DB().QueryRow("SELECT rows FROM ga4_import_runs WHERE CAST(export_date AS VARCHAR)='2024-03-05'").Scan(&n); err != nil || n != 0 {
		t.Errorf("import run rows = %d, err %v; want a run with rows=0", n, err)
	}
}

// Scenario: Import of 5M rows meets the time budget
// Real size (5,000,000 events, 50,000 sessions) against the 10 minute budget on the DuckDB adapter;
// skipped under -short. The BigQuery path is covered by the e2e/staging run.
func TestImportOf5MRowsMeetsTheTimeBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("5M-row import skipped in -short mode")
	}
	dir := t.TempDir()
	gen, err := sql.Open("duckdb", "")
	if err != nil {
		t.Fatal(err)
	}
	defer gen.Close()
	out := filepath.Join(dir, "events_20240305.parquet")
	_, err = gen.Exec(`COPY (SELECT '20240305' AS event_date,
	  CAST(1709632800000000 + (i % 3600) * 1000000 AS BIGINT) AS event_timestamp,
	  'view_item' AS event_name,
	  'u' || CAST(i % 25000 AS VARCHAR) AS user_pseudo_id,
	  [ {'key':'ga_session_id','value':{'string_value':CAST(NULL AS VARCHAR),'int_value':CAST((i % 50000) // 25000 + 1 AS BIGINT)}},
	    {'key':'source','value':{'string_value':'google','int_value':CAST(NULL AS BIGINT)}},
	    {'key':'medium','value':{'string_value':'cpc','int_value':CAST(NULL AS BIGINT)}},
	    {'key':'campaign','value':{'string_value':'c','int_value':CAST(NULL AS BIGINT)}},
	    {'key':'page_location','value':{'string_value':'https://agora.test/p/1?a=b','int_value':CAST(NULL AS BIGINT)}} ] AS event_params,
	  {'name':'c','medium':'cpc','source':'google'} AS traffic_source
	  FROM range(5000000) t(i)) TO '` + out + `' (FORMAT PARQUET)`)
	if err != nil {
		t.Fatal(err)
	}
	w := open(t)
	start := time.Now()
	res, err := run(t, w, dir, d1)
	if err != nil {
		t.Fatal(err)
	}
	if el := time.Since(start); el > 10*time.Minute {
		t.Errorf("import took %v, budget 10m", el)
	}
	if res.Rows != 5_000_000 {
		t.Errorf("Result.Rows = %d, want 5000000 (events read)", res.Rows)
	}
	var n int64
	if err := w.DB().QueryRow("SELECT rows FROM ga4_import_runs WHERE CAST(export_date AS VARCHAR)='2024-03-05'").Scan(&n); err != nil || n != 5_000_000 {
		t.Errorf("import run rows = %d, err %v", n, err)
	}
}
