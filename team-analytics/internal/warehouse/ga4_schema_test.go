package warehouse_test

import (
	"testing"

	"github.com/buidangphuc/team-analytics/internal/warehouse"
)

func colMap(cols []warehouse.Column) map[string]warehouse.Column {
	m := map[string]warehouse.Column{}
	for _, c := range cols {
		m[c.Name] = c
	}
	return m
}

// Scenario: Schema parity holds (GA4SessionsSchema is the single source, incl. export_date and ended_at)
func TestSchemaParityHolds_GA4SessionsSchemaColumns(t *testing.T) {
	m := colMap(warehouse.GA4SessionsSchema)
	if len(m) != len(warehouse.GA4SessionsSchema) || len(m) == 0 {
		t.Fatalf("GA4SessionsSchema empty or has duplicate names: %+v", warehouse.GA4SessionsSchema)
	}
	for _, n := range []string{"export_date", "user_pseudo_id", "session_id", "source", "medium", "campaign", "landing_page", "ts", "ended_at"} {
		if _, ok := m[n]; !ok {
			t.Errorf("GA4SessionsSchema missing %q", n)
		}
	}
	if c := m["export_date"]; c.DuckDBType != "DATE" || c.BigQueryType != "DATE" {
		t.Errorf("export_date types = %q/%q, want DATE/DATE", c.DuckDBType, c.BigQueryType)
	}
	for _, n := range []string{"ts", "ended_at"} {
		if c := m[n]; c.DuckDBType != "TIMESTAMP" || c.BigQueryType != "TIMESTAMP" {
			t.Errorf("%s types = %q/%q, want TIMESTAMP", n, c.DuckDBType, c.BigQueryType)
		}
	}
}

// Scenario: Schema parity holds (GA4EventsDailySchema)
func TestSchemaParityHolds_GA4EventsDailySchemaColumns(t *testing.T) {
	m := colMap(warehouse.GA4EventsDailySchema)
	for _, n := range []string{"day", "event_name", "events"} {
		if _, ok := m[n]; !ok {
			t.Errorf("GA4EventsDailySchema missing %q", n)
		}
	}
	if len(m) != 3 {
		t.Errorf("GA4EventsDailySchema has %d columns, want 3", len(m))
	}
	if c := m["day"]; c.DuckDBType != "DATE" || c.BigQueryType != "DATE" {
		t.Errorf("day types = %q/%q, want DATE", c.DuckDBType, c.BigQueryType)
	}
	if c := m["events"]; c.DuckDBType != "BIGINT" || c.BigQueryType != "INT64" {
		t.Errorf("events types = %q/%q, want BIGINT/INT64", c.DuckDBType, c.BigQueryType)
	}
}

// Scenario: Schema parity holds (ga4_import_runs: export_date, rows, finished_at)
func TestSchemaParityHolds_GA4ImportRunsSchemaColumns(t *testing.T) {
	m := colMap(warehouse.GA4ImportRunsSchema)
	for _, n := range []string{"export_date", "rows", "finished_at"} {
		if _, ok := m[n]; !ok {
			t.Errorf("GA4ImportRunsSchema missing %q", n)
		}
	}
	if warehouse.GA4SessionsTableName != "ga4_sessions" || warehouse.GA4EventsDailyTableName != "ga4_events_daily" || warehouse.GA4ImportRunsTableName != "ga4_import_runs" {
		t.Errorf("table name constants wrong: %q %q %q", warehouse.GA4SessionsTableName, warehouse.GA4EventsDailyTableName, warehouse.GA4ImportRunsTableName)
	}
}
