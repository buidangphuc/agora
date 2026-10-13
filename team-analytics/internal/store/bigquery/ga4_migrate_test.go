package bigquery

// Tests for wh-ga4 (BigQuery). Seam the implementer must add, mirroring Migrate:
//
//	func GA4MigrationDDL(project, dataset string) []string        // for ga4_sessions, ga4_events_daily, ga4_import_runs:
//	                                                              // CREATE TABLE IF NOT EXISTS + ALTER TABLE ... ADD COLUMN IF NOT EXISTS per column
//	func MigrateGA4(ctx context.Context, exec DDLExecutor, project, dataset string) error
//
// Writer.Open must call MigrateGA4 and return its error (no import run is recorded by Open).

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/buidangphuc/team-analytics/internal/store"
)

func joined(stmts []string) string { return strings.Join(stmts, "\n") }

// Scenario: Schema parity holds (BigQuery DDL carries every column and BQ type from the schemas)
func TestSchemaParityHolds_BigQueryGA4DDLDerivedFromSchemas(t *testing.T) {
	ddl := GA4MigrationDDL("p", "d")
	for table, schema := range map[string][]store.Column{
		"ga4_sessions":     store.GA4SessionsSchema,
		"ga4_events_daily": store.GA4EventsDailySchema,
		"ga4_import_runs":  store.GA4ImportRunsSchema,
	} {
		var mine []string
		for _, s := range ddl {
			if strings.Contains(s, "`p.d."+table+"`") {
				mine = append(mine, s)
			}
		}
		if len(mine) == 0 {
			t.Errorf("no DDL for %s", table)
			continue
		}
		all := joined(mine)
		for _, c := range schema {
			if !strings.Contains(all, c.Name+" "+c.BigQueryType) {
				t.Errorf("%s: DDL lacks %q %s", table, c.Name, c.BigQueryType)
			}
		}
		if !strings.Contains(strings.ToUpper(all), "CREATE TABLE IF NOT EXISTS") {
			t.Errorf("%s: no CREATE TABLE IF NOT EXISTS", table)
		}
	}
}

// Scenario: Migration is idempotent (only IF NOT EXISTS, never DROP; ADD COLUMN IF NOT EXISTS for export_date and ended_at)
func TestMigrationIsIdempotent_BigQueryGA4DDLIsReRunnable(t *testing.T) {
	ex := &recExec{}
	for i := 0; i < 2; i++ {
		if err := MigrateGA4(context.Background(), ex, "p", "d"); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
	}
	if len(ex.stmts) == 0 || len(ex.stmts)%2 != 0 {
		t.Fatalf("expected the same statements on both runs, got %d", len(ex.stmts))
	}
	for _, s := range ex.stmts {
		u := strings.ToUpper(s)
		if strings.Contains(u, "DROP") || !strings.Contains(u, "IF NOT EXISTS") {
			t.Errorf("non-idempotent statement: %s", s)
		}
	}
	for _, col := range []string{"export_date DATE", "ended_at TIMESTAMP"} {
		found := false
		for _, s := range ex.stmts {
			found = found || (strings.Contains(strings.ToUpper(s), "ADD COLUMN IF NOT EXISTS") &&
				strings.Contains(s, "`p.d.ga4_sessions`") && strings.Contains(s, col))
		}
		if !found {
			t.Errorf("no ADD COLUMN IF NOT EXISTS for ga4_sessions %q", col)
		}
	}
}

// Scenario: A BigQuery migration failure is surfaced (wraps cause, names the table, stops at first error)
func TestBigQueryMigrationFailureIsSurfaced_GA4(t *testing.T) {
	boom := errors.New("permission denied")
	ex := &recExec{err: boom}
	err := MigrateGA4(context.Background(), ex, "p", "d")
	if err == nil || !errors.Is(err, boom) {
		t.Fatalf("err = %v, want wrapping %v", err, boom)
	}
	if !strings.Contains(err.Error(), "ga4_") {
		t.Errorf("error does not name the ga4 table: %v", err)
	}
	if len(ex.stmts) != 1 {
		t.Errorf("issued %d statements after failure, want 1", len(ex.stmts))
	}
}
