package bigquery

// Tests for wh-bigquery. They target a seam the implementer must add (no seam
// exists today because ensureSchema talks to *bigquery.Client directly):
//
//	type DDLExecutor interface{ Exec(ctx context.Context, stmt string) error }
//	func MigrationDDL(project, dataset, table string) []string   // ALTER TABLE ... ADD COLUMN IF NOT EXISTS, one per new column
//	func Migrate(ctx context.Context, exec DDLExecutor, project, dataset, table string) error
//	func GA4ViewDDL(project, dataset, table string) string       // CREATE OR REPLACE VIEW ga4_events
//	func GA4ViewColumns() []string                               // projected column names of that view
//
// Writer.Open must call Migrate (via a client-backed DDLExecutor) before ingest.

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/buidangphuc/team-analytics/internal/warehouse"
	"github.com/buidangphuc/team-analytics/internal/warehouse/duckdb"
)

var newSet = map[string]bool{"item_list_id": true, "position": true, "event_group_id": true, "transaction_id": true, "currency": true, "value": true}

// newTrackingColumns are the six new columns in warehouse.Schema order.
var newTrackingColumns = func() []string {
	var out []string
	for _, c := range warehouse.Schema {
		if newSet[c.Name] {
			out = append(out, c.Name)
		}
	}
	return out
}()

type recExec struct {
	stmts []string
	err   error
}

func (r *recExec) Exec(_ context.Context, s string) error {
	r.stmts = append(r.stmts, s)
	return r.err
}

// Scenario: BigQuery migration is idempotent
func TestBigQueryMigrationIsIdempotent_IssuesAddColumnIfNotExists(t *testing.T) {
	ex := &recExec{}
	for i := 0; i < 2; i++ { // re-run on an already-migrated table
		if err := Migrate(context.Background(), ex, "p", "d", "tracking_events"); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
	}
	if len(ex.stmts) != 2*len(newTrackingColumns) {
		t.Fatalf("got %d statements, want %d", len(ex.stmts), 2*len(newTrackingColumns))
	}
	for i, col := range newTrackingColumns {
		found := false
		for _, s := range ex.stmts[i : i+1] { // statements follow warehouse.Schema order
			u := strings.ToUpper(s)
			if strings.Contains(u, "ALTER TABLE") && strings.Contains(u, "ADD COLUMN IF NOT EXISTS") &&
				strings.Contains(s, "`p.d.tracking_events`") && strings.Contains(s, col+" ") {
				found = true
			}
		}
		if !found {
			t.Errorf("no ADD COLUMN IF NOT EXISTS statement for %q: %v", col, ex.stmts)
		}
	}
	for _, s := range ex.stmts {
		if strings.Contains(strings.ToUpper(s), "DROP") || !strings.Contains(strings.ToUpper(s), "IF NOT EXISTS") {
			t.Errorf("non-idempotent statement: %s", s)
		}
	}
}

// Scenario: BigQuery migration failure is surfaced
func TestBigQueryMigrationFailureIsSurfaced_NamesTable(t *testing.T) {
	boom := errors.New("permission denied")
	err := Migrate(context.Background(), &recExec{err: boom}, "p", "d", "tracking_events")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, boom) {
		t.Errorf("error does not wrap cause: %v", err)
	}
	if !strings.Contains(err.Error(), "tracking_events") {
		t.Errorf("error does not name the table: %v", err)
	}
}

// Scenario: BigQuery migration failure is surfaced (stops at first failure, nothing further issued)
func TestBigQueryMigrationFailureIsSurfaced_StopsAtFirstError(t *testing.T) {
	ex := &recExec{err: errors.New("x")}
	_ = Migrate(context.Background(), ex, "p", "d", "tracking_events")
	if len(ex.stmts) != 1 {
		t.Fatalf("issued %d statements after failure, want 1", len(ex.stmts))
	}
}

// Scenario: Schema parity (BigQuery table columns == warehouse.Schema names, in order)
func TestSchemaParity_BigQueryColumnsMatchWarehouseSchema(t *testing.T) {
	s, err := bqSchema()
	if err != nil {
		t.Fatal(err)
	}
	if len(s) != len(warehouse.Schema) {
		t.Fatalf("bq has %d columns, warehouse.Schema %d", len(s), len(warehouse.Schema))
	}
	for i, c := range warehouse.Schema {
		if s[i].Name != c.Name {
			t.Errorf("col %d: bq %q vs schema %q", i, s[i].Name, c.Name)
		}
	}
	for _, n := range newTrackingColumns {
		found := false
		for _, c := range warehouse.Schema {
			found = found || c.Name == n
		}
		if !found {
			t.Errorf("warehouse.Schema missing %q", n)
		}
	}
}

// Scenario: Schema parity (ga4_events view exposes same projected names in both engines)
func TestSchemaParity_GA4EventsViewColumnsMatchDuckDB(t *testing.T) {
	ctx := context.Background()
	w, err := duckdb.Open(ctx, filepath.Join(t.TempDir(), "w.duckdb"))
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	rows, err := w.DB().QueryContext(ctx, "SELECT * FROM ga4_events LIMIT 0")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	duck, err := rows.Columns()
	if err != nil {
		t.Fatal(err)
	}
	bq := GA4ViewColumns()
	if strings.Join(duck, ",") != strings.Join(bq, ",") {
		t.Fatalf("ga4_events columns differ\nduckdb:   %v\nbigquery: %v", duck, bq)
	}
	hasIndex := false
	for _, c := range bq {
		if c == "position" {
			t.Error("view must project position AS index, not position")
		}
		hasIndex = hasIndex || c == "index"
	}
	if !hasIndex {
		t.Errorf("view lacks projected column index: %v", bq)
	}
	ddl := GA4ViewDDL("p", "d", "tracking_events")
	if !strings.Contains(strings.ToUpper(ddl), "CREATE OR REPLACE VIEW") || !strings.Contains(ddl, "ga4_events") {
		t.Errorf("bad view DDL: %s", ddl)
	}
}
