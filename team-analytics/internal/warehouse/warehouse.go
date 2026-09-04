// Package warehouse defines the driver-neutral analytics-sink seam: the
// TrackingRecord the consumer produces, the canonical table Schema both adapters
// agree on, and the WarehouseWriter interface implemented by the DuckDB
// (local/test) and BigQuery (prod) adapters. Selecting an adapter is a boot-time
// concern (see internal/bootstrap.OpenWarehouse) so this package stays free of
// any adapter import — adapters depend on warehouse, never the reverse.
package warehouse

import (
	"context"
	"time"
)

// TrackingRecord is the flat, engine-neutral row the consumer maps every
// TrackingEvent (+ its EventEnvelope context) into. Both adapters write exactly
// these columns; nothing engine-specific leaks into it.
type TrackingRecord struct {
	// EventID is the envelope event_id (uuid); carried so a downstream job can
	// dedupe under at-least-once delivery.
	EventID string
	// EventType is the normalized lowercase EventType name (e.g. "view",
	// "click", "add_to_cart", "impression").
	EventType string
	ListingID string
	SessionID string
	// AnonymousID is the cookie/device id (NOT an authenticated user id).
	AnonymousID string
	PagePath    string
	Referrer    string
	// Position is the 1-based rank within a result set (0 when N/A).
	Position uint32
	SearchQuery string
	// OccurredAt is the envelope occurred_at (producer clock), UTC.
	OccurredAt time.Time
	// PrincipalID / PrincipalType come from the envelope principal (ADR-0003);
	// empty/anonymous when the actor was not authenticated.
	PrincipalID   string
	PrincipalType string
	// Properties is the open-ended extension bag, persisted as a JSON column.
	Properties map[string]string
}

// Column is one entry of the canonical warehouse schema. The DuckDB and BigQuery
// SQL types are deliberately kept to the intersection both engines support so a
// TrackingRecord round-trips identically through either adapter (design.md,
// "DuckDB ↔ BigQuery SQL parity"). Adapters build their own CREATE TABLE DDL
// from this single ordered list, so a column can never drift between them.
type Column struct {
	Name         string
	DuckDBType   string
	BigQueryType string
}

// Schema is the ordered, append-only column list for the `tracking_events`
// table. It is the parity anchor: both adapters derive their DDL from it and a
// parity test asserts neither adapter diverges (warehouse_test.go).
var Schema = []Column{
	{"event_id", "VARCHAR", "STRING"},
	{"event_type", "VARCHAR", "STRING"},
	{"listing_id", "VARCHAR", "STRING"},
	{"session_id", "VARCHAR", "STRING"},
	{"anonymous_id", "VARCHAR", "STRING"},
	{"page_path", "VARCHAR", "STRING"},
	{"referrer", "VARCHAR", "STRING"},
	{"position", "INTEGER", "INT64"},
	{"search_query", "VARCHAR", "STRING"},
	{"occurred_at", "TIMESTAMP", "TIMESTAMP"},
	{"principal_id", "VARCHAR", "STRING"},
	{"principal_type", "VARCHAR", "STRING"},
	{"properties", "JSON", "JSON"},
}

// ColumnNames returns the ordered column names of the canonical schema.
func ColumnNames() []string {
	names := make([]string, len(Schema))
	for i, c := range Schema {
		names[i] = c.Name
	}
	return names
}

// TableName is the single append-only table both adapters write to.
const TableName = "tracking_events"

// WarehouseWriter is the swap seam: the consumer writes batches through this one
// interface, and WAREHOUSE_DRIVER picks the concrete adapter at boot. Switching
// drivers changes nothing on the consume/unmarshal/map path — only the env value
// and the adapter behind this interface (spec: "swappable behind a
// WarehouseWriter seam").
type WarehouseWriter interface {
	// Write durably appends a batch of records. It MUST return an error rather
	// than partially/silently dropping rows: the caller commits Kafka offsets
	// only after Write returns nil (at-least-once).
	Write(ctx context.Context, batch []*TrackingRecord) error
	// Close flushes and releases the underlying handle.
	Close() error
}
