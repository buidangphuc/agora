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
	Position    uint32
	SearchQuery string
	// OccurredAt is the envelope occurred_at (producer clock), UTC.
	OccurredAt time.Time
	// PrincipalID / PrincipalType come from the envelope principal (ADR-0003);
	// empty/anonymous when the actor was not authenticated.
	PrincipalID   string
	PrincipalType string
	// Properties is the open-ended extension bag, persisted as a JSON column.
	Properties map[string]string
	// PlacementID is the placement slot (e.g. "home_feed", "similar_items").
	PlacementID string
	// ImpressionID is the unique impression uuid linking downstream interactions.
	ImpressionID string
	// ModelVersion is the model generation identifier.
	ModelVersion  string
	Currency      string
	Value         int64
	Price         int64
	Quantity      uint32
	TransactionID string
	Coupon        string
	ItemCategory  string
	ItemListID    string
	ItemListName  string
	EventGroupID  string
	ShippingTier  string
	PaymentType   string
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
	{"placement_id", "VARCHAR", "STRING"},
	{"impression_id", "VARCHAR", "STRING"},
	{"model_version", "VARCHAR", "STRING"},
	{"currency", "VARCHAR", "STRING"},
	{"value", "BIGINT", "INT64"},
	{"price", "BIGINT", "INT64"},
	{"quantity", "INTEGER", "INT64"},
	{"transaction_id", "VARCHAR", "STRING"},
	{"coupon", "VARCHAR", "STRING"},
	{"item_category", "VARCHAR", "STRING"},
	{"item_list_id", "VARCHAR", "STRING"},
	{"item_list_name", "VARCHAR", "STRING"},
	{"event_group_id", "VARCHAR", "STRING"},
	{"shipping_tier", "VARCHAR", "STRING"},
	{"payment_type", "VARCHAR", "STRING"},
	// ingested_at is the sink's own clock at write time (tracking-ingest-integrity
	// D4); occurred_at stays the edge receive time. Null on rows written before
	// the column existed.
	{"ingested_at", "TIMESTAMP", "TIMESTAMP"},
}

// Stitching view names (tracking-ingest-integrity D5). The principal_type
// literal for an authenticated visitor is "user" (consumer.principalTypeName).
const (
	IdentityViewName = "tracking_identity"
	ResolvedViewName = "tracking_events_resolved"
)

// CountersTableName is the per-UTC-hour table of what the tracking sink absorbed
// (analytics-data-quality D1): undecodable messages and skipped duplicates.
const CountersTableName = "tracking_ingest_counters"

// IngestCounterWriter is implemented by adapters that keep the ingest counters
// (the DuckDB adapter). The consumer counts a message it cannot decode through it.
// Duplicates are counted by the adapter itself inside Write.
type IngestCounterWriter interface {
	// RecordDecodeFailures adds n to the decode_failures counter of the UTC hour
	// containing at.
	RecordDecodeFailures(ctx context.Context, at time.Time, n int64) error
}

// ColumnNames returns the ordered column names of the canonical schema.
func ColumnNames() []string {
	names := make([]string, len(Schema))
	for i, c := range Schema {
		names[i] = c.Name
	}
	return names
}

// OrderFactRecord is one row in the order_facts warehouse table, representing
// an authoritative purchased line item emitted by team-order on PAID transition (ADR-0013).
type OrderFactRecord struct {
	EventID    string
	OrderID    string
	ListingID  string
	VariantID  string
	SellerID   string
	Quantity   int32
	UnitPrice  int64
	Currency   string
	OccurredAt time.Time
	Status     string
	// BuyerID is the buyer's user id from OrderPaidEvent.buyer_id; empty is stored
	// as NULL (unattributed), as are rows ingested before the column existed.
	BuyerID string
}

// OrderFactsTableName is the canonical table for authoritative line-item purchase facts.
const OrderFactsTableName = "order_facts"

// OrderFactsSchema is the canonical schema for order_facts with DuckDB/BigQuery parity.
var OrderFactsSchema = []Column{
	{"event_id", "VARCHAR", "STRING"},
	{"order_id", "VARCHAR", "STRING"},
	{"listing_id", "VARCHAR", "STRING"},
	{"variant_id", "VARCHAR", "STRING"},
	{"seller_id", "VARCHAR", "STRING"},
	{"quantity", "INTEGER", "INT64"},
	{"unit_price", "BIGINT", "INT64"},
	{"currency", "VARCHAR", "STRING"},
	{"occurred_at", "TIMESTAMP", "TIMESTAMP"},
	{"status", "VARCHAR", "STRING"},
	{"buyer_id", "VARCHAR", "STRING"},
}

// OrderFactsColumnNames returns the ordered column names of order_facts.
func OrderFactsColumnNames() []string {
	names := make([]string, len(OrderFactsSchema))
	for i, c := range OrderFactsSchema {
		names[i] = c.Name
	}
	return names
}

// TableName is the single append-only table both adapters write to for tracking.
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
	// WriteOrderFacts appends a batch of order line item facts.
	WriteOrderFacts(ctx context.Context, batch []*OrderFactRecord) error
	// Close flushes and releases the underlying handle.
	Close() error
}

// ListingSellerRecord maps a listing to its owning seller. Tracking events carry
// only a listing id, so the seller funnel joins through this table. It is
// derived from team-domain's ListingChanged events (listing.events).
type ListingSellerRecord struct {
	ListingID string
	SellerID  string
	// UpdatedAt is the envelope occurred_at of the event that produced the row.
	UpdatedAt time.Time
}

// ListingSellersTableName is the listing -> seller mapping table (DuckDB only).
const ListingSellersTableName = "listing_sellers"

// ListingSellerWriter is implemented by adapters that keep the listing -> seller
// mapping (the DuckDB adapter, which also serves the seller queries).
type ListingSellerWriter interface {
	// UpsertListingSellers idempotently inserts or refreshes the mappings in one
	// transaction. Mappings are never deleted: a deleted listing keeps its row so
	// historical tracking events stay attributable to the seller.
	UpsertListingSellers(ctx context.Context, batch []*ListingSellerRecord) error
}

// Engagement fact names stored in engagement_facts.fact (engagement-fact-events D3).
const (
	FactFavoriteAdded    = "favorite_added"
	FactFavoriteRemoved  = "favorite_removed"
	FactSellerFollowed   = "seller_followed"
	FactSellerUnfollowed = "seller_unfollowed"
	FactReviewCreated    = "review_created"
)

// EngagementFactRecord is one row of engagement_facts: a server-truth preference
// signal published by team-engagement. Fields that do not apply to the fact are
// empty (Rating is 0 and stored as NULL for every fact but review_created).
type EngagementFactRecord struct {
	EventID    string
	Fact       string
	UserID     string
	ListingID  string
	SellerID   string
	Rating     int32
	OccurredAt time.Time
}

// Engagement table and view names (DuckDB only).
const (
	EngagementFactsTableName = "engagement_facts"
	FavoritesCurrentViewName = "favorites_current"
	FollowsCurrentViewName   = "follows_current"
)

// EngagementFactWriter is implemented by adapters that keep engagement_facts
// (the DuckDB adapter).
type EngagementFactWriter interface {
	// WriteEngagementFacts appends the batch in one transaction, idempotently on
	// event_id, stamping ingested_at with the sink clock.
	WriteEngagementFacts(ctx context.Context, batch []*EngagementFactRecord) error
}
