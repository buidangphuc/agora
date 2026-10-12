package duckdb

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/buidangphuc/team-analytics/internal/store"
)

// tableColumns returns (name, type) pairs of a table in declared order.
func tableColumns(t *testing.T, w *Writer, table string) (names, types []string) {
	t.Helper()
	rows, err := w.DB().Query("SELECT column_name, data_type FROM information_schema.columns WHERE table_name = ? ORDER BY ordinal_position", table)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var n, ty string
		if err := rows.Scan(&n, &ty); err != nil {
			t.Fatal(err)
		}
		names = append(names, n)
		types = append(types, strings.ToUpper(ty))
	}
	return
}

func checkParity(t *testing.T, w *Writer, table string, schema []store.Column) {
	t.Helper()
	names, types := tableColumns(t, w, table)
	if len(names) != len(schema) {
		t.Fatalf("%s: duckdb has %d columns %v, schema has %d", table, len(names), names, len(schema))
	}
	for i, c := range schema {
		if names[i] != c.Name {
			t.Errorf("%s col %d: duckdb %q, schema %q", table, i, names[i], c.Name)
		}
		if types[i] != c.DuckDBType {
			t.Errorf("%s col %q: duckdb type %q, schema %q", table, c.Name, types[i], c.DuckDBType)
		}
	}
}

// Scenario: Schema parity holds (DuckDB tables match the schemas: names, order, types)
func TestSchemaParityHolds_DuckDBGA4TablesMatchSchemas(t *testing.T) {
	w, err := Open(context.Background(), filepath.Join(t.TempDir(), "w.duckdb"))
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	checkParity(t, w, store.GA4SessionsTableName, store.GA4SessionsSchema)
	checkParity(t, w, store.GA4EventsDailyTableName, store.GA4EventsDailySchema)
	checkParity(t, w, store.GA4ImportRunsTableName, store.GA4ImportRunsSchema)
}

// Scenario: Migration is idempotent (open twice on a db that already has ga4_sessions)
func TestMigrationIsIdempotent_DuckDBReopenKeepsColumnsAndRows(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "w.duckdb")
	w1, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w1.DB().Exec("INSERT INTO ga4_sessions (export_date, user_pseudo_id, session_id, source, medium) VALUES (DATE '2026-10-01', '1.2', 's1', 'google', 'cpc')"); err != nil {
		t.Fatal(err)
	}
	before, _ := tableColumns(t, w1, "ga4_sessions")
	w1.Close()
	for i := 0; i < 2; i++ {
		w, err := Open(ctx, path)
		if err != nil {
			t.Fatalf("open %d: %v", i, err)
		}
		after, _ := tableColumns(t, w, "ga4_sessions")
		if strings.Join(before, ",") != strings.Join(after, ",") {
			t.Errorf("open %d changed columns: %v -> %v", i, before, after)
		}
		if n := count(t, w, "ga4_sessions"); n != 1 {
			t.Errorf("open %d: rows = %d, want 1", i, n)
		}
		w.Close()
	}
}

// Scenario: Migration is idempotent (a legacy ga4_sessions without export_date/ended_at gains them)
func TestMigrationIsIdempotent_DuckDBLegacyGA4SessionsGainsColumns(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "w.duckdb")
	w, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{
		"DROP TABLE ga4_sessions",
		"CREATE TABLE ga4_sessions (user_pseudo_id VARCHAR, session_id VARCHAR, source VARCHAR, medium VARCHAR, campaign VARCHAR, landing_page VARCHAR, ts TIMESTAMP)",
		"INSERT INTO ga4_sessions VALUES ('1.2','s1','google','cpc','c','/p/1', TIMESTAMP '2026-10-01 10:00:00')",
	} {
		if _, err := w.DB().Exec(s); err != nil {
			t.Fatal(err)
		}
	}
	w.Close()
	w2, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("open legacy: %v", err)
	}
	defer w2.Close()
	checkParity(t, w2, store.GA4SessionsTableName, store.GA4SessionsSchema)
	if n := count(t, w2, "ga4_sessions"); n != 1 {
		t.Errorf("legacy row lost: %d", n)
	}
}

// Scenario: ga4_sessions carries export_date (replace-by-date keeps other dates; export_date is DATE-typed key)
func TestGA4SessionsCarriesExportDate_ReplaceByDateLeavesOtherDates(t *testing.T) {
	ctx := context.Background()
	w, err := Open(ctx, filepath.Join(t.TempDir(), "w.duckdb"))
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	for _, s := range []string{
		"INSERT INTO ga4_sessions (export_date, user_pseudo_id, session_id, source, medium) VALUES (DATE '2026-10-01','1.1','a','google','cpc'), (DATE '2026-10-02','1.1','b','direct','(none)')",
		"DELETE FROM ga4_sessions WHERE export_date = DATE '2026-10-01'",
		"INSERT INTO ga4_sessions (export_date, user_pseudo_id, session_id, source, medium) VALUES (DATE '2026-10-01','1.1','a2','bing','organic')",
	} {
		if _, err := w.DB().Exec(s); err != nil {
			t.Fatal(err)
		}
	}
	var d2, d1 string
	if err := w.DB().QueryRow("SELECT session_id FROM ga4_sessions WHERE export_date = DATE '2026-10-02'").Scan(&d2); err != nil || d2 != "b" {
		t.Errorf("D2 row = %q err=%v", d2, err)
	}
	if err := w.DB().QueryRow("SELECT session_id FROM ga4_sessions WHERE export_date = DATE '2026-10-01'").Scan(&d1); err != nil || d1 != "a2" {
		t.Errorf("D1 row = %q err=%v", d1, err)
	}
	var nulls int
	if err := w.DB().QueryRow("SELECT COUNT(*) FROM ga4_sessions WHERE export_date IS NULL").Scan(&nulls); err != nil || nulls != 0 {
		t.Errorf("null export_date rows = %d err=%v", nulls, err)
	}
}
