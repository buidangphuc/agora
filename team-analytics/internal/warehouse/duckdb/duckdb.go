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

	// Registers the "duckdb" database/sql driver. Requires CGO at build time;
	// the worker image builds it in Docker/CI (Go is not on the host).
	_ "github.com/marcboeker/go-duckdb"

	"github.com/buidangphuc/team-analytics/internal/warehouse"
)

// Writer appends TrackingRecords to a DuckDB `tracking_events` table.
type Writer struct {
	db   *sql.DB
	path string
}

// Open dials (opens/creates) the DuckDB file at path and ensures the schema.
func Open(ctx context.Context, path string) (*Writer, error) {
	db, err := sql.Open("duckdb", path)
	if err != nil {
		return nil, fmt.Errorf("open duckdb %q: %w", path, err)
	}
	w := &Writer{db: db, path: path}
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

func (w *Writer) ensureSchema(ctx context.Context) error {
	if _, err := w.db.ExecContext(ctx, createTableDDL()); err != nil {
		return fmt.Errorf("ensure %s table: %w", warehouse.TableName, err)
	}
	return nil
}

// insertSQL is the parameterized append for one row, column order == Schema.
var insertSQL = buildInsertSQL()

func buildInsertSQL() string {
	names := warehouse.ColumnNames()
	ph := make([]string, len(names))
	for i := range names {
		ph[i] = "?"
	}
	return fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES (%s)",
		warehouse.TableName, strings.Join(names, ", "), strings.Join(ph, ", "),
	)
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
