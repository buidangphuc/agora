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
	Impressions int64
	Views       int64
	Adds        int64 // add-to-cart count
	Orders      int64 // distinct orders (purchases)
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
}
