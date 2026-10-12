package main

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/buidangphuc/team-analytics/internal/warehouse/duckdb"
	_ "github.com/marcboeker/go-duckdb"
)

func sessionCount(t *testing.T, path string) int {
	t.Helper()
	w, err := duckdb.Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	var n int
	if err := w.DB().QueryRow("SELECT count(*) FROM ga4_sessions").Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func env(t *testing.T, kv map[string]string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "w.duckdb")
	t.Setenv("WAREHOUSE_DRIVER", "duckdb")
	t.Setenv("DUCKDB_PATH", path)
	for _, k := range []string{"GA4_EXPORT_SOURCE", "GA4_EXPORT_DATASET", "GA4_EXPORT_FIXTURE_DIR", "GA4_RECONCILE_MAX_DIFF_RATIO"} {
		t.Setenv(k, "")
	}
	for k, v := range kv {
		t.Setenv(k, v)
	}
	return path
}

// Scenario: The job is a no-op when unconfigured
func TestJobIsANoOpWhenUnconfigured(t *testing.T) {
	path := env(t, nil) // GA4_EXPORT_DATASET unset
	var out, errb bytes.Buffer
	if code := run(context.Background(), []string{"--date", "2024-03-05"}, &out, &errb); code != 0 {
		t.Fatalf("exit %d, stderr %q", code, errb.String())
	}
	if n := sessionCount(t, path); n != 0 {
		t.Errorf("wrote %d rows, want 0", n)
	}
}

// Scenario: Invalid threshold is rejected at startup
func TestInvalidThresholdIsRejectedAtStartup(t *testing.T) {
	env(t, map[string]string{"GA4_RECONCILE_MAX_DIFF_RATIO": "-1", "GA4_EXPORT_DATASET": "fixture", "GA4_EXPORT_SOURCE": "fixture"})
	var out, errb bytes.Buffer
	if code := run(context.Background(), []string{"--date", "2024-03-05"}, &out, &errb); code == 0 {
		t.Fatal("want non-zero exit")
	}
	if !strings.Contains(errb.String(), "GA4_RECONCILE_MAX_DIFF_RATIO") {
		t.Errorf("stderr must name the setting, got %q", errb.String())
	}
}

// Scenario: Import of a fixture day produces sessions (through the CLI, fixture source)
func TestCLIImportsAFixtureDay(t *testing.T) {
	dir := t.TempDir()
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
	path := env(t, map[string]string{"GA4_EXPORT_SOURCE": "fixture", "GA4_EXPORT_DATASET": "fixture", "GA4_EXPORT_FIXTURE_DIR": dir})
	var out, errb bytes.Buffer
	if code := run(context.Background(), []string{"--date", "2024-03-05"}, &out, &errb); code != 0 {
		t.Fatalf("exit %d, stderr %q", code, errb.String())
	}
	if n := sessionCount(t, path); n != 2 {
		t.Errorf("sessions = %d, want 2", n)
	}
	// missing date: non-zero exit naming the date
	errb.Reset()
	if code := run(context.Background(), []string{"--date", "2024-03-06"}, &out, &errb); code == 0 || !strings.Contains(errb.String(), "2024-03-06") {
		t.Errorf("missing table: want non-zero exit naming the date, stderr %q", errb.String())
	}
}
