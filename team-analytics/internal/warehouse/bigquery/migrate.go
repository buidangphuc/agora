package bigquery

import (
	"context"
	"fmt"
	"strings"

	"cloud.google.com/go/bigquery"

	"github.com/buidangphuc/team-analytics/internal/warehouse"
)

// DDLExecutor runs one DDL statement against the warehouse.
type DDLExecutor interface {
	Exec(ctx context.Context, stmt string) error
}

// ga4NewColumns are the columns added by the GA4 datalayer change.
var ga4NewColumns = map[string]bool{
	"item_list_id": true, "position": true, "event_group_id": true,
	"transaction_id": true, "currency": true, "value": true,
}

// MigrationDDL returns one idempotent ADD COLUMN statement per new column, in
// warehouse.Schema order.
func MigrationDDL(project, dataset, table string) []string {
	var out []string
	for _, c := range warehouse.Schema {
		if ga4NewColumns[c.Name] {
			out = append(out, fmt.Sprintf("ALTER TABLE `%s.%s.%s` ADD COLUMN IF NOT EXISTS %s %s",
				project, dataset, table, c.Name, c.BigQueryType))
		}
	}
	return out
}

// Migrate applies MigrationDDL, stopping at the first failure.
func Migrate(ctx context.Context, exec DDLExecutor, project, dataset, table string) error {
	for _, stmt := range MigrationDDL(project, dataset, table) {
		if err := exec.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("migrate table %s.%s.%s: %w", project, dataset, table, err)
		}
	}
	return nil
}

var ga4ViewProjection = []struct{ expr, name string }{
	{"event_id", "event_id"},
	{"CASE event_type WHEN 'view' THEN 'view_item' WHEN 'click' THEN 'select_item' WHEN 'impression' THEN 'view_item_list' ELSE event_type END", "event_name"},
	{"listing_id", "item_id"},
	{"session_id", "session_id"},
	{"anonymous_id", "anonymous_id"},
	{"page_path", "page_path"},
	{"referrer", "page_referrer"},
	{"position", "index"},
	{"search_query", "search_term"},
	{"occurred_at", "occurred_at"},
	{"principal_id", "principal_id"},
	{"principal_type", "principal_type"},
	{"properties", "properties"},
	{"placement_id", "placement_id"},
	{"impression_id", "impression_id"},
	{"model_version", "model_version"},
	{"currency", "currency"},
	{"value", "value"},
	{"price", "price"},
	{"quantity", "quantity"},
	{"transaction_id", "transaction_id"},
	{"coupon", "coupon"},
	{"item_category", "item_category"},
	{"item_list_id", "item_list_id"},
	{"item_list_name", "item_list_name"},
	{"event_group_id", "event_group_id"},
	{"shipping_tier", "shipping_tier"},
	{"payment_type", "payment_type"},
}

// GA4ViewColumns lists the projected column names of the ga4_events view.
func GA4ViewColumns() []string {
	out := make([]string, len(ga4ViewProjection))
	for i, p := range ga4ViewProjection {
		out[i] = p.name
	}
	return out
}

// GA4ViewDDL is the BigQuery equivalent of the DuckDB ga4_events view.
func GA4ViewDDL(project, dataset, table string) string {
	cols := make([]string, len(ga4ViewProjection))
	for i, p := range ga4ViewProjection {
		if p.expr == p.name {
			cols[i] = "  " + p.name
		} else {
			cols[i] = fmt.Sprintf("  %s AS `%s`", p.expr, p.name)
		}
	}
	return fmt.Sprintf("CREATE OR REPLACE VIEW `%s.%s.ga4_events` AS\nSELECT\n%s\nFROM `%s.%s.%s`",
		project, dataset, strings.Join(cols, ",\n"), project, dataset, table)
}

// clientExec adapts a *bigquery.Client to DDLExecutor.
type clientExec struct{ client *bigquery.Client }

func (e clientExec) Exec(ctx context.Context, stmt string) error {
	job, err := e.client.Query(stmt).Run(ctx)
	if err != nil {
		return err
	}
	st, err := job.Wait(ctx)
	if err != nil {
		return err
	}
	return st.Err()
}

var ga4Tables = []struct {
	name   string
	schema []warehouse.Column
}{
	{warehouse.GA4SessionsTableName, warehouse.GA4SessionsSchema},
	{warehouse.GA4EventsDailyTableName, warehouse.GA4EventsDailySchema},
	{warehouse.GA4ImportRunsTableName, warehouse.GA4ImportRunsSchema},
}

// GA4MigrationDDL returns idempotent CREATE TABLE IF NOT EXISTS plus
// ADD COLUMN IF NOT EXISTS statements for the GA4 export-import tables.
func GA4MigrationDDL(project, dataset string) []string {
	var out []string
	for _, t := range ga4Tables {
		fq := fmt.Sprintf("`%s.%s.%s`", project, dataset, t.name)
		cols := make([]string, len(t.schema))
		for i, c := range t.schema {
			cols[i] = c.Name + " " + c.BigQueryType
		}
		out = append(out, fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (%s)", fq, strings.Join(cols, ", ")))
		for _, c := range t.schema {
			out = append(out, fmt.Sprintf("ALTER TABLE %s ADD COLUMN IF NOT EXISTS %s %s", fq, c.Name, c.BigQueryType))
		}
	}
	return out
}

// MigrateGA4 applies GA4MigrationDDL, stopping at the first failure.
func MigrateGA4(ctx context.Context, exec DDLExecutor, project, dataset string) error {
	for _, stmt := range GA4MigrationDDL(project, dataset) {
		if err := exec.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("migrate ga4 tables in %s.%s: %w (stmt: %s)", project, dataset, err, stmt)
		}
	}
	return nil
}
