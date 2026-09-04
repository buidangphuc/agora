package warehouse_test

import (
	"context"
	"testing"
	"time"

	"github.com/buidangphuc/team-analytics/internal/warehouse"
	"github.com/buidangphuc/team-analytics/internal/warehouse/fake"
)

// allowedDuckDB / allowedBigQuery are the intersection types the two engines
// agree on (design.md). A column typed outside these sets is drift.
var (
	allowedDuckDB   = map[string]bool{"VARCHAR": true, "INTEGER": true, "TIMESTAMP": true, "JSON": true}
	allowedBigQuery = map[string]bool{"STRING": true, "INT64": true, "TIMESTAMP": true, "JSON": true}
)

// TestSchemaParity is the DuckDB↔BigQuery parity guard: both adapters derive
// their DDL from warehouse.Schema, so verifying every column carries a valid
// type for BOTH engines proves neither adapter can diverge.
func TestSchemaParity(t *testing.T) {
	if len(warehouse.Schema) == 0 {
		t.Fatal("warehouse.Schema is empty")
	}
	seen := map[string]bool{}
	for _, c := range warehouse.Schema {
		if c.Name == "" {
			t.Fatalf("column with empty name: %+v", c)
		}
		if seen[c.Name] {
			t.Fatalf("duplicate column %q", c.Name)
		}
		seen[c.Name] = true
		if !allowedDuckDB[c.DuckDBType] {
			t.Errorf("column %q: DuckDB type %q outside the shared intersection", c.Name, c.DuckDBType)
		}
		if !allowedBigQuery[c.BigQueryType] {
			t.Errorf("column %q: BigQuery type %q outside the shared intersection", c.Name, c.BigQueryType)
		}
	}
	// The behavioral fields the spec scenario requires must be present.
	for _, required := range []string{
		"event_id", "event_type", "listing_id", "session_id", "page_path",
		"occurred_at", "principal_id", "principal_type",
	} {
		if !seen[required] {
			t.Errorf("canonical schema missing required column %q", required)
		}
	}
}

func TestColumnNamesMatchSchemaOrder(t *testing.T) {
	names := warehouse.ColumnNames()
	if len(names) != len(warehouse.Schema) {
		t.Fatalf("ColumnNames len %d != Schema len %d", len(names), len(warehouse.Schema))
	}
	for i, c := range warehouse.Schema {
		if names[i] != c.Name {
			t.Errorf("ColumnNames[%d] = %q, want %q", i, names[i], c.Name)
		}
	}
}

// TestFakeRoundTrip exercises the WarehouseWriter seam through the in-memory
// fake: a batch written is a batch retrievable, in order.
func TestFakeRoundTrip(t *testing.T) {
	w := fake.New()
	batch := []*warehouse.TrackingRecord{
		{EventID: "e1", EventType: "view", ListingID: "prod-1", OccurredAt: time.Unix(1, 0).UTC()},
		{EventID: "e2", EventType: "click", ListingID: "prod-2", OccurredAt: time.Unix(2, 0).UTC()},
	}
	if err := w.Write(context.Background(), batch); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got := w.Rows()
	if len(got) != 2 {
		t.Fatalf("wrote 2 rows, fake has %d", len(got))
	}
	if got[0].EventID != "e1" || got[1].EventID != "e2" {
		t.Errorf("row order not preserved: %q, %q", got[0].EventID, got[1].EventID)
	}
	if w.Batches() != 1 {
		t.Errorf("expected 1 flush, got %d", w.Batches())
	}
	if err := w.Close(); err != nil || !w.Closed() {
		t.Errorf("Close failed: err=%v closed=%v", err, w.Closed())
	}
}

// TestWriterInterface asserts the fake (and, by the compile-time asserts in each
// adapter file, the real adapters) satisfy the seam.
func TestWriterInterface(t *testing.T) {
	var _ warehouse.WarehouseWriter = fake.New()
}
