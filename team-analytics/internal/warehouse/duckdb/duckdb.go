// Package duckdb is the local/test WarehouseWriter adapter. It appends
// TrackingRecords into an embedded DuckDB database (columnar storage, analyst-
// queryable, no external service) and can export the table to columnar Parquet.
// It owns its own DDL, derived from warehouse.Schema so it can never drift from
// the BigQuery adapter.
package duckdb

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	// Registers the "duckdb" database/sql driver. Requires CGO at build time;
	// the worker image builds it in Docker/CI (Go is not on the host).
	_ "github.com/marcboeker/go-duckdb"

	"github.com/buidangphuc/team-analytics/internal/warehouse"
)

// Writer appends TrackingRecords to a DuckDB `tracking_events` table.
type Writer struct {
	db   *sql.DB
	path string
	now  func() time.Time // ingested_at clock; overridable in tests
}

// Open dials (opens/creates) the DuckDB file at path and ensures the schema.
func Open(ctx context.Context, path string) (*Writer, error) {
	db, err := sql.Open("duckdb", path)
	if err != nil {
		return nil, fmt.Errorf("open duckdb %q: %w", path, err)
	}
	w := &Writer{db: db, path: path, now: time.Now}
	if err := w.ensureSchema(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return w, nil
}

// createTableDDL builds `CREATE TABLE IF NOT EXISTS tracking_events (...)` from
// the canonical schema using each column's DuckDB type.
func createTableDDL() string {
	cols := make([]string, len(warehouse.Schema))
	for i, c := range warehouse.Schema {
		cols[i] = fmt.Sprintf("%s %s", c.Name, c.DuckDBType)
	}
	return fmt.Sprintf(
		"CREATE TABLE IF NOT EXISTS %s (\n  %s\n)",
		warehouse.TableName, strings.Join(cols, ",\n  "),
	)
}

func createOrderFactsTableDDL() string {
	cols := make([]string, len(warehouse.OrderFactsSchema))
	for i, c := range warehouse.OrderFactsSchema {
		cols[i] = fmt.Sprintf("%s %s", c.Name, c.DuckDBType)
	}
	return fmt.Sprintf(
		"CREATE TABLE IF NOT EXISTS %s (\n  %s\n)",
		warehouse.OrderFactsTableName, strings.Join(cols, ",\n  "),
	)
}

func (w *Writer) ensureSchema(ctx context.Context) error {
	if _, err := w.db.ExecContext(ctx, createTableDDL()); err != nil {
		return fmt.Errorf("ensure %s table: %w", warehouse.TableName, err)
	}

	// Idempotent column migrations for existing databases (Gap G6)
	for _, c := range warehouse.Schema {
		alterSQL := fmt.Sprintf("ALTER TABLE %s ADD COLUMN IF NOT EXISTS %s %s", warehouse.TableName, c.Name, c.DuckDBType)
		if _, err := w.db.ExecContext(ctx, alterSQL); err != nil {
			return fmt.Errorf("migrate column %s: %w", c.Name, err)
		}
	}

	if _, err := w.db.ExecContext(ctx, createOrderFactsTableDDL()); err != nil {
		return fmt.Errorf("ensure %s table: %w", warehouse.OrderFactsTableName, err)
	}

	for _, c := range warehouse.OrderFactsSchema {
		alterSQL := fmt.Sprintf("ALTER TABLE %s ADD COLUMN IF NOT EXISTS %s %s", warehouse.OrderFactsTableName, c.Name, c.DuckDBType)
		if _, err := w.db.ExecContext(ctx, alterSQL); err != nil {
			return fmt.Errorf("migrate order_facts column %s: %w", c.Name, err)
		}
	}

	listingSellersDDL := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
  listing_id VARCHAR PRIMARY KEY,
  seller_id  VARCHAR NOT NULL,
  updated_at TIMESTAMP NOT NULL
)`, warehouse.ListingSellersTableName)
	if _, err := w.db.ExecContext(ctx, listingSellersDDL); err != nil {
		return fmt.Errorf("ensure %s table: %w", warehouse.ListingSellersTableName, err)
	}

	// Create or replace standard ga4_events view
	createViewSQL := fmt.Sprintf(`CREATE OR REPLACE VIEW ga4_events AS
SELECT
  event_id,
  CASE event_type
    WHEN 'view' THEN 'view_item'
    WHEN 'click' THEN 'select_item'
    WHEN 'impression' THEN 'view_item_list'
    ELSE event_type
  END AS event_name,
  listing_id AS item_id,
  session_id,
  anonymous_id,
  page_path,
  referrer AS page_referrer,
  position AS index,
  search_query AS search_term,
  occurred_at,
  principal_id,
  principal_type,
  properties,
  placement_id,
  impression_id,
  model_version,
  currency,
  value,
  price,
  quantity,
  transaction_id,
  coupon,
  item_category,
  item_list_id,
  item_list_name,
  event_group_id,
  shipping_tier,
  payment_type
FROM %s`, warehouse.TableName)

	if _, err := w.db.ExecContext(ctx, createViewSQL); err != nil {
		return fmt.Errorf("ensure ga4_events view: %w", err)
	}

	// Stitching views (tracking-ingest-integrity D5). Created after the column
	// migrations so t.* includes ingested_at.
	for _, ddl := range []string{identityViewDDL, resolvedViewDDL} {
		if _, err := w.db.ExecContext(ctx, ddl); err != nil {
			return fmt.Errorf("ensure stitching view: %w", err)
		}
	}

	return nil
}

// identityViewDDL maps each anonymous id seen on USER-principal events to that
// principal. The anonymous id comes from the beacon body (unauthenticated), so an
// id seen with more than one account is ambiguous and is not stitched: a logged-in
// user cannot claim another visitor's anonymous history by replaying its id.
var identityViewDDL = fmt.Sprintf(`CREATE OR REPLACE VIEW %s AS
SELECT anonymous_id, any_value(principal_id) AS principal_id
FROM %s
WHERE principal_type = 'user' AND anonymous_id <> ''
GROUP BY anonymous_id
HAVING count(DISTINCT principal_id) = 1`, warehouse.IdentityViewName, warehouse.TableName)

// resolvedViewDDL adds user_key: the event's own USER principal, else the
// stitched principal, else "anon:<anonymous_id>".
var resolvedViewDDL = fmt.Sprintf(`CREATE OR REPLACE VIEW %s AS
SELECT t.*, CASE
  WHEN t.principal_type = 'user' THEN t.principal_id
  WHEN i.principal_id IS NOT NULL THEN i.principal_id
  ELSE 'anon:' || t.anonymous_id END AS user_key
FROM %s t LEFT JOIN %s i USING (anonymous_id)`,
	warehouse.ResolvedViewName, warehouse.TableName, warehouse.IdentityViewName)

// insertSQL is the parameterized append for one row, column order == Schema.
var insertSQL = buildInsertSQL()
var insertOrderFactsSQL = buildInsertOrderFactsSQL()

// buildIdempotentInsert builds an anti-join append keyed on event_id:
// INSERT ... SELECT CAST(? AS type)... WHERE NOT EXISTS (same event_id). Kafka
// delivery is at-least-once, so a redelivered event must not create a second
// row. An anti-join (not a unique index) is used because existing volumes may
// already hold duplicates, which would make CREATE UNIQUE INDEX fail. Rows of
// one batch run in one transaction, so duplicates inside a batch are also
// skipped. The event_id is bound twice: last placeholder feeds the NOT EXISTS.
func buildIdempotentInsert(table string, names, types []string) string {
	sel := make([]string, len(names))
	for i := range names {
		sel[i] = fmt.Sprintf("CAST(? AS %s)", types[i])
	}
	return fmt.Sprintf(
		"INSERT INTO %s (%s) SELECT %s WHERE NOT EXISTS (SELECT 1 FROM %s WHERE event_id = ?)",
		table, strings.Join(names, ", "), strings.Join(sel, ", "), table,
	)
}

func buildInsertSQL() string {
	types := make([]string, len(warehouse.Schema))
	for i, c := range warehouse.Schema {
		types[i] = c.DuckDBType
	}
	return buildIdempotentInsert(warehouse.TableName, warehouse.ColumnNames(), types)
}

func buildInsertOrderFactsSQL() string {
	types := make([]string, len(warehouse.OrderFactsSchema))
	for i, c := range warehouse.OrderFactsSchema {
		types[i] = c.DuckDBType
	}
	return buildIdempotentInsert(warehouse.OrderFactsTableName, warehouse.OrderFactsColumnNames(), types)
}

// Write appends the batch in one transaction — DuckDB strongly prefers bulk
// appends over per-row autocommit. The whole batch commits or rolls back, so a
// flush is all-or-nothing and the caller can safely commit offsets after nil.
func (w *Writer) Write(ctx context.Context, batch []*warehouse.TrackingRecord) error {
	if len(batch) == 0 {
		return nil
	}
	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	stmt, err := tx.PrepareContext(ctx, insertSQL)
	if err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("prepare insert: %w", err)
	}
	defer stmt.Close()

	ingestedAt := w.now().UTC()
	for _, r := range batch {
		props, err := marshalProperties(r.Properties)
		if err != nil {
			_ = tx.Rollback()
			return err
		}
		if _, err := stmt.ExecContext(ctx,
			r.EventID,
			r.EventType,
			r.ListingID,
			r.SessionID,
			r.AnonymousID,
			r.PagePath,
			r.Referrer,
			int64(r.Position),
			r.SearchQuery,
			r.OccurredAt,
			r.PrincipalID,
			r.PrincipalType,
			props,
			r.PlacementID,
			r.ImpressionID,
			r.ModelVersion,
			r.Currency,
			r.Value,
			r.Price,
			int64(r.Quantity),
			r.TransactionID,
			r.Coupon,
			r.ItemCategory,
			r.ItemListID,
			r.ItemListName,
			r.EventGroupID,
			r.ShippingTier,
			r.PaymentType,
			ingestedAt,
			r.EventID, // NOT EXISTS dedupe key
		); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("insert row %s: %w", r.EventID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit batch: %w", err)
	}
	return nil
}

// WriteOrderFacts appends a batch of order line items in one transaction.
func (w *Writer) WriteOrderFacts(ctx context.Context, batch []*warehouse.OrderFactRecord) error {
	if len(batch) == 0 {
		return nil
	}
	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	stmt, err := tx.PrepareContext(ctx, insertOrderFactsSQL)
	if err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("prepare insert order facts: %w", err)
	}
	defer stmt.Close()

	for _, r := range batch {
		if _, err := stmt.ExecContext(ctx,
			r.EventID,
			r.OrderID,
			r.ListingID,
			r.VariantID,
			r.SellerID,
			int64(r.Quantity),
			r.UnitPrice,
			r.Currency,
			r.OccurredAt,
			r.Status,
			r.EventID, // NOT EXISTS dedupe key
		); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("insert order fact row %s: %w", r.EventID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit order facts batch: %w", err)
	}
	return nil
}

// upsertListingSellerSQL refreshes a mapping only when the incoming event is not
// older than the stored one, so an out-of-order redelivery cannot regress it.
var upsertListingSellerSQL = fmt.Sprintf(
	`INSERT INTO %[1]s (listing_id, seller_id, updated_at) VALUES (?, ?, ?)
ON CONFLICT (listing_id) DO UPDATE SET seller_id = excluded.seller_id, updated_at = excluded.updated_at
WHERE excluded.updated_at >= %[1]s.updated_at`, warehouse.ListingSellersTableName)

// UpsertListingSellers idempotently upserts listing -> seller mappings in one
// transaction (all-or-nothing, so the caller can commit offsets after nil).
func (w *Writer) UpsertListingSellers(ctx context.Context, batch []*warehouse.ListingSellerRecord) error {
	if len(batch) == 0 {
		return nil
	}
	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	stmt, err := tx.PrepareContext(ctx, upsertListingSellerSQL)
	if err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("prepare upsert listing_sellers: %w", err)
	}
	defer stmt.Close()
	for _, r := range batch {
		if _, err := stmt.ExecContext(ctx, r.ListingID, r.SellerID, r.UpdatedAt.UTC()); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("upsert listing_sellers %s: %w", r.ListingID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit listing_sellers batch: %w", err)
	}
	return nil
}

// ExportParquet writes the whole table out as columnar Parquet at dst. DuckDB's
// COPY produces analyst-/Spark-readable Parquet — the shape the later
// recommendation job consumes.
func (w *Writer) ExportParquet(ctx context.Context, dst string) error {
	q := fmt.Sprintf("COPY %s TO '%s' (FORMAT PARQUET)", warehouse.TableName, dst)
	if _, err := w.db.ExecContext(ctx, q); err != nil {
		return fmt.Errorf("export parquet to %q: %w", dst, err)
	}
	return nil
}

// Close closes the underlying database handle.
func (w *Writer) Close() error { return w.db.Close() }

// DB exposes the underlying *sql.DB so the read-only query layer (seller
// dashboards) can run aggregations over the same in-process handle instead of
// opening a second connection to the file (DuckDB single-writer file lock).
// Reads and the batch appends serialize through database/sql — the query side
// never writes.
func (w *Writer) DB() *sql.DB { return w.db }

func marshalProperties(p map[string]string) (string, error) {
	if len(p) == 0 {
		return "{}", nil
	}
	b, err := json.Marshal(p)
	if err != nil {
		return "", fmt.Errorf("marshal properties: %w", err)
	}
	return string(b), nil
}

// compile-time assertion that the adapter satisfies the seam.
var _ warehouse.WarehouseWriter = (*Writer)(nil)
var _ warehouse.ListingSellerWriter = (*Writer)(nil)
