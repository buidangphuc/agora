// Package query is the read-only seller-analytics query layer: the gRPC
// AnalyticsQueryService (service.go) over a Repository that aggregates the
// warehouse `tracking_events` table the Kafka consumer populates. It NEVER
// writes — the TrackingEvent pipeline is the only writer.
//
// Schema adaptation (honest note): `tracking_events` (see internal/warehouse)
// has no first-class seller_id, order, revenue or SKU columns — it is a
// behavioral-event table (event_type ∈ view|click|add_to_cart|impression) plus
// an open-ended `properties` JSON bag (TrackingEvent.properties, documented as
// the extension point for attributes without a contract change). The seller
// dashboard fields therefore ride in `properties`:
//   - seller_id : properties.seller_id   (row is attributed to a seller)
//   - order_id  : properties.order_id    (non-empty marks a purchase/order)
//   - revenue   : properties.revenue     (minor units, integer string)
//   - sku       : properties.sku         (top-SKU grouping key)
//   - units     : properties.units       (units sold, integer string)
//
// Funnel impressions/views/adds come straight from event_type; orders are the
// distinct order_id count (no dedicated order event type exists in the enum).
// If a future migration promotes these to real columns, only the DuckDB SQL
// (duckdb.go) changes — this interface and the service stay put.
package query

import (
	"context"
	"time"
)

// Funnel is the seller conversion funnel over a [from, to] window.
type Funnel struct {
	Impressions    int64
	Views          int64
	Adds           int64 // add-to-cart count
	Orders         int64 // distinct orders (purchases)
	BeginCheckouts int64
	Purchases      int64
}

// DayRevenue is one calendar day's revenue and order count (minor units).
type DayRevenue struct {
	Day        string // ISO date, e.g. "2026-09-04" (UTC)
	Revenue    int64
	OrderCount int64
}

// TopSku is one top-selling SKU's revenue and units over the window.
type TopSku struct {
	SKU       string
	ListingID string
	Revenue   int64 // minor units
	UnitsSold int64
}

// Breakdown is revenue split by day and by top-selling SKU.
type Breakdown struct {
	Days    []DayRevenue
	TopSkus []TopSku
}

// DailyPoint represents a single day's quantile forecast.
type DailyPoint struct {
	Date string
	P10  float64
	P50  float64
	P90  float64
}

// ForecastResult contains the multi-day probabilistic demand prediction.
type ForecastResult struct {
	SellerID      string
	ListingID     string
	ModelVersion  string
	IsColdStart   bool
	DailyForecast []DailyPoint
}

// OrderSummary is the platform-wide paid-order aggregate over a window.
type OrderSummary struct {
	OrderCount int64 // distinct paid orders
	GMV        int64 // SUM(quantity * unit_price), minor units
}

// RecentOrder is one paid order, aggregated over its line items.
type RecentOrder struct {
	OrderID  string
	SellerID string // first seller on the order when it spans several
	Total    int64  // minor units
	PaidAt   time.Time
}

// Repository is the read-only warehouse seam the query service depends on. Two
// implementations exist: DuckDBRepository (real SQL over tracking_events) and
// MemoryRepository (in-memory, used by the unit tests so they need no live
// warehouse).
type Repository interface {
	// SellerFunnel returns impression→view→add→order counts for sellerID over
	// [from, to] (inclusive). An empty/degenerate window yields all-zero counts.
	SellerFunnel(ctx context.Context, sellerID string, from, to time.Time) (Funnel, error)
	// RevenueBreakdown returns per-day revenue and the top-N SKUs by revenue for
	// sellerID over [from, to] (inclusive). Empty window yields empty slices.
	RevenueBreakdown(ctx context.Context, sellerID string, from, to time.Time, topN int) (Breakdown, error)
	// DemandForecast returns probabilistic demand quantiles (p10, p50, p90) for a listing over horizonDays.
	DemandForecast(ctx context.Context, sellerID, listingID string, horizonDays int) (ForecastResult, error)
	// PlatformOrderSummary aggregates every seller's paid orders with
	// occurred_at >= since. An empty table yields the zero summary.
	PlatformOrderSummary(ctx context.Context, since time.Time) (OrderSummary, error)
	// RecentOrders returns up to limit paid orders, newest first.
	RecentOrders(ctx context.Context, limit int) ([]RecentOrder, error)
}

// TypeQuality is one event type's quality numbers over the report window.
type TypeQuality struct {
	EventType string
	Events    int64
	Visitors  int64 // distinct user_key from tracking_events_resolved
	// MissingListingRatio is the share of events with an empty listing_id; it is
	// only measured for listing-scoped types (ListingScoped) and 0 otherwise.
	MissingListingRatio float64
	ListingScoped       bool
}

// TrackingQualityData is the raw tracking-stream health measured over a window;
// the service turns it into a status (analytics-data-quality D2).
type TrackingQualityData struct {
	Types []TypeQuality // ordered by event type
	// HasLag is false when no row in the window has an ingested_at.
	HasLag         bool
	LagP50Seconds  float64
	LagP95Seconds  float64
	LastIngestedAt time.Time // zero when no row in the window has an ingested_at
	DecodeFailures int64
	Duplicates     int64
}

// QualityRepository is implemented by repositories that can measure the tracking
// stream (the DuckDB one). The window is [since, until] on occurred_at; counters
// are summed over the UTC hours overlapping it.
type QualityRepository interface {
	TrackingQuality(ctx context.Context, since, until time.Time) (TrackingQualityData, error)
}

// PerformanceRow is one (placement, model_version) line of the recommendation
// performance report (recsys-online-evaluation D1).
type PerformanceRow struct {
	PlacementID     string
	ModelVersion    string
	Impressions     int64 // distinct impression_id
	ItemImpressions int64 // impression events
	Clicks          int64
	AddToCarts      int64
	Purchases       int64
}

// PerformanceRepository is implemented by repositories that can attribute
// recommendation outcomes (the DuckDB one). Impressions and clicks are taken
// from [since, until] on occurred_at; add-to-carts and purchases count when they
// fall within attributionHours after a click from the same impression.
type PerformanceRepository interface {
	RecommendationPerformance(ctx context.Context, since, until time.Time, attributionHours int) ([]PerformanceRow, error)
}
