// Package bigquery is the prod WarehouseWriter adapter. It streams TrackingRecord
// batches into a BigQuery `tracking_events` table, date-partitioned and
// clustered for the analytical scans the recommendation engine will run. It owns
// its own DDL, derived from warehouse.Schema so it can never drift from the
// DuckDB adapter.
//
// BigQuery needs credentials/emulation not available in local e2e, so this
// adapter is exercised by unit tests + the schema-parity test rather than the
// live local stack (design.md, "BigQuery in CI"). DuckDB is the local/CI driver.
package bigquery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"cloud.google.com/go/bigquery"
	"google.golang.org/api/googleapi"

	"github.com/buidangphuc/team-analytics/internal/warehouse"
)

// Writer streams TrackingRecords into a BigQuery table via the insert-all API.
type Writer struct {
	client   *bigquery.Client
	inserter *bigquery.Inserter
	dataset  string
	table    string
}

// Config carries the BigQuery target coordinates.
type Config struct {
	Project string
	Dataset string
	Table   string
}

// Open creates the BigQuery client and ensures the dataset/table exist.
func Open(ctx context.Context, cfg Config) (*Writer, error) {
	client, err := bigquery.NewClient(ctx, cfg.Project)
	if err != nil {
		return nil, fmt.Errorf("bigquery client for project %q: %w", cfg.Project, err)
	}
	w := &Writer{
		client:  client,
		dataset: cfg.Dataset,
		table:   cfg.Table,
	}
	if err := w.ensureSchema(ctx); err != nil {
		_ = client.Close()
		return nil, err
	}
	w.inserter = client.Dataset(cfg.Dataset).Table(cfg.Table).Inserter()
	return w, nil
}

// bqSchema maps the canonical warehouse.Schema to a BigQuery schema using each
// column's declared BigQuery type.
func bqSchema() (bigquery.Schema, error) {
	s := make(bigquery.Schema, 0, len(warehouse.Schema))
	for _, c := range warehouse.Schema {
		ft, err := fieldType(c.BigQueryType)
		if err != nil {
			return nil, fmt.Errorf("column %s: %w", c.Name, err)
		}
		s = append(s, &bigquery.FieldSchema{Name: c.Name, Type: ft})
	}
	return s, nil
}

func fieldType(t string) (bigquery.FieldType, error) {
	switch t {
	case "STRING":
		return bigquery.StringFieldType, nil
	case "INT64":
		return bigquery.IntegerFieldType, nil
	case "TIMESTAMP":
		return bigquery.TimestampFieldType, nil
	case "JSON":
		return bigquery.JSONFieldType, nil
	default:
		return "", fmt.Errorf("unmapped BigQuery type %q", t)
	}
}

func (w *Writer) ensureSchema(ctx context.Context) error {
	schema, err := bqSchema()
	if err != nil {
		return err
	}
	meta := &bigquery.TableMetadata{
		Schema: schema,
		// Partition by ingest date for cheap pruned scans; cluster by the columns
		// the recommendation engine filters on most.
		TimePartitioning: &bigquery.TimePartitioning{
			Type:  bigquery.DayPartitioningType,
			Field: "occurred_at",
		},
		Clustering: &bigquery.Clustering{Fields: []string{"event_type", "listing_id"}},
	}
	err = w.client.Dataset(w.dataset).Table(w.table).Create(ctx, meta)
	if err != nil && !isAlreadyExists(err) {
		return fmt.Errorf("ensure %s.%s table: %w", w.dataset, w.table, err)
	}
	return nil
}

// Write streams the batch into BigQuery. The insert id is the envelope event_id,
// so BigQuery best-effort de-duplicates re-delivered rows on reprocessing.
func (w *Writer) Write(ctx context.Context, batch []*warehouse.TrackingRecord) error {
	if len(batch) == 0 {
		return nil
	}
	rows := make([]*rowSaver, len(batch))
	for i, r := range batch {
		rows[i] = &rowSaver{rec: r}
	}
	if err := w.inserter.Put(ctx, rows); err != nil {
		return fmt.Errorf("bigquery insert batch of %d: %w", len(batch), err)
	}
	return nil
}

// Close closes the BigQuery client.
func (w *Writer) Close() error { return w.client.Close() }

// rowSaver adapts a TrackingRecord to the BigQuery insert API, keeping the row
// shape aligned with the canonical schema and setting InsertID for dedup.
type rowSaver struct{ rec *warehouse.TrackingRecord }

func (s *rowSaver) Save() (map[string]bigquery.Value, string, error) {
	props := "{}"
	if len(s.rec.Properties) > 0 {
		b, err := json.Marshal(s.rec.Properties)
		if err != nil {
			return nil, "", fmt.Errorf("marshal properties: %w", err)
		}
		props = string(b)
	}
	row := map[string]bigquery.Value{
		"event_id":       s.rec.EventID,
		"event_type":     s.rec.EventType,
		"listing_id":     s.rec.ListingID,
		"session_id":     s.rec.SessionID,
		"anonymous_id":   s.rec.AnonymousID,
		"page_path":      s.rec.PagePath,
		"referrer":       s.rec.Referrer,
		"position":       int64(s.rec.Position),
		"search_query":   s.rec.SearchQuery,
		"occurred_at":    s.rec.OccurredAt,
		"principal_id":   s.rec.PrincipalID,
		"principal_type": s.rec.PrincipalType,
		"properties":     props,
	}
	return row, s.rec.EventID, nil
}

func isAlreadyExists(err error) bool {
	var apiErr *googleapi.Error
	if errors.As(err, &apiErr) {
		return apiErr.Code == http.StatusConflict
	}
	return false
}

// compile-time assertions.
var (
	_ warehouse.WarehouseWriter = (*Writer)(nil)
	_ bigquery.ValueSaver       = (*rowSaver)(nil)
)
