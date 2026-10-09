package query

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"time"

	"github.com/buidangphuc/team-analytics/internal/warehouse"
)

// DuckDBRepository answers the seller queries with real aggregations over the
// DuckDB `tracking_events` table. It is read-only: every statement is a SELECT.
// It reuses the writer's *sql.DB (see duckdb.Writer.DB) so it shares the single
// in-process handle rather than opening a second, lock-conflicting connection.
type DuckDBRepository struct {
	db *sql.DB
}

// NewDuckDBRepository wraps an already-open handle to the warehouse database.
func NewDuckDBRepository(db *sql.DB) *DuckDBRepository {
	return &DuckDBRepository{db: db}
}

// SellerFunnel aggregates impressions, views, adds from tracking_events and
// distinct orders from order_facts (ADR-0013). Tracking events carry no seller
// id, so they are attributed through listing_sellers (listing_id -> seller_id);
// events on a listing with no known seller are excluded from every seller funnel.
func (r *DuckDBRepository) SellerFunnel(ctx context.Context, sellerID string, from, to time.Time) (Funnel, error) {
	trackingQ := fmt.Sprintf(`
SELECT
  COUNT(*) FILTER (WHERE t.event_type = 'impression') AS impressions,
  COUNT(*) FILTER (WHERE t.event_type = 'view')       AS views,
  COUNT(*) FILTER (WHERE t.event_type = 'add_to_cart') AS adds,
  COUNT(*) FILTER (WHERE t.event_type = 'begin_checkout') AS begin_checkouts,
  COUNT(*) FILTER (WHERE t.event_type = 'purchase') AS purchases
FROM %s t
JOIN %s ls ON ls.listing_id = t.listing_id
WHERE ls.seller_id = ?
  AND t.occurred_at >= ? AND t.occurred_at <= ?`,
		warehouse.TableName, warehouse.ListingSellersTableName)

	var f Funnel
	row := r.db.QueryRowContext(ctx, trackingQ, sellerID, from.UTC(), to.UTC())
	if err := row.Scan(&f.Impressions, &f.Views, &f.Adds, &f.BeginCheckouts, &f.Purchases); err != nil {
		return Funnel{}, fmt.Errorf("seller funnel tracking query: %w", err)
	}

	ordersQ := fmt.Sprintf(`
SELECT
  COUNT(DISTINCT order_id) AS orders
FROM %s
WHERE seller_id = ?
  AND occurred_at >= ? AND occurred_at <= ?`,
		warehouse.OrderFactsTableName)

	orderRow := r.db.QueryRowContext(ctx, ordersQ, sellerID, from.UTC(), to.UTC())
	if err := orderRow.Scan(&f.Orders); err != nil {
		return Funnel{}, fmt.Errorf("seller funnel orders query: %w", err)
	}

	return f, nil
}

// RevenueBreakdown returns per-day revenue/order-count and the top-N SKUs by
// revenue for sellerID over [from, to] directly from the order_facts table.
func (r *DuckDBRepository) RevenueBreakdown(ctx context.Context, sellerID string, from, to time.Time, topN int) (Breakdown, error) {
	if topN <= 0 {
		topN = defaultTopSKULimit
	}
	var b Breakdown

	dayQ := fmt.Sprintf(`
SELECT
  strftime(occurred_at, '%%Y-%%m-%%d') AS day,
  COALESCE(SUM(quantity * unit_price), 0) AS revenue,
  COUNT(DISTINCT order_id)            AS order_count
FROM %s
WHERE seller_id = ?
  AND occurred_at >= ? AND occurred_at <= ?
GROUP BY 1
ORDER BY 1`,
		warehouse.OrderFactsTableName)

	dayRows, err := r.db.QueryContext(ctx, dayQ, sellerID, from.UTC(), to.UTC())
	if err != nil {
		return Breakdown{}, fmt.Errorf("revenue-by-day query: %w", err)
	}
	defer dayRows.Close()
	for dayRows.Next() {
		var d DayRevenue
		if err := dayRows.Scan(&d.Day, &d.Revenue, &d.OrderCount); err != nil {
			return Breakdown{}, fmt.Errorf("scan day revenue: %w", err)
		}
		b.Days = append(b.Days, d)
	}
	if err := dayRows.Err(); err != nil {
		return Breakdown{}, fmt.Errorf("iterate day revenue: %w", err)
	}

	skuQ := fmt.Sprintf(`
SELECT
  COALESCE(NULLIF(variant_id, ''), listing_id) AS sku,
  listing_id                                   AS listing_id,
  COALESCE(SUM(quantity * unit_price), 0)      AS revenue,
  COALESCE(SUM(quantity), 0)                   AS units_sold
FROM %s
WHERE seller_id = ?
  AND occurred_at >= ? AND occurred_at <= ?
GROUP BY 1, 2
ORDER BY revenue DESC, sku ASC
LIMIT ?`,
		warehouse.OrderFactsTableName)

	skuRows, err := r.db.QueryContext(ctx, skuQ, sellerID, from.UTC(), to.UTC(), topN)
	if err != nil {
		return Breakdown{}, fmt.Errorf("top-sku query: %w", err)
	}
	defer skuRows.Close()
	for skuRows.Next() {
		var s TopSku
		if err := skuRows.Scan(&s.SKU, &s.ListingID, &s.Revenue, &s.UnitsSold); err != nil {
			return Breakdown{}, fmt.Errorf("scan top sku: %w", err)
		}
		b.TopSkus = append(b.TopSkus, s)
	}
	if err := skuRows.Err(); err != nil {
		return Breakdown{}, fmt.Errorf("iterate top sku: %w", err)
	}
	return b, nil
}

// DemandForecast calculates probabilistic daily demand quantiles from order_facts history or floor.
func (r *DuckDBRepository) DemandForecast(ctx context.Context, sellerID, listingID string, horizonDays int) (ForecastResult, error) {
	if horizonDays <= 0 {
		horizonDays = 28
	}

	q := fmt.Sprintf(`
SELECT
  COALESCE(SUM(quantity), 0) AS total_units,
  COALESCE(AVG(quantity), 0) AS avg_daily,
  COALESCE(STDDEV_POP(quantity), 0) AS std_daily
FROM %s
WHERE seller_id = ?
  AND (listing_id = ? OR variant_id = ?)
`, warehouse.OrderFactsTableName)

	var totalUnits int64
	var avgDaily, stdDaily float64
	row := r.db.QueryRowContext(ctx, q, sellerID, listingID, listingID)
	if err := row.Scan(&totalUnits, &avgDaily, &stdDaily); err != nil {
		// Non-fatal, fallback to defaults
		totalUnits = 0
		avgDaily = 2.0
		stdDaily = 1.0
	}

	isColdStart := totalUnits == 0
	if avgDaily <= 0 {
		avgDaily = 2.0
	}
	if stdDaily <= 0 {
		stdDaily = math.Max(0.5, avgDaily*0.3)
	}

	points := make([]DailyPoint, horizonDays)
	now := time.Now().UTC()
	for i := 0; i < horizonDays; i++ {
		dt := now.AddDate(0, 0, i+1).Format("2006-01-02")
		p10 := math.Max(0, avgDaily-1.28*stdDaily)
		p50 := avgDaily
		p90 := avgDaily + 1.28*stdDaily

		points[i] = DailyPoint{
			Date: dt,
			P10:  float64(int(p10*10)) / 10.0,
			P50:  float64(int(p50*10)) / 10.0,
			P90:  float64(int(p90*10)) / 10.0,
		}
	}

	return ForecastResult{
		SellerID:      sellerID,
		ListingID:     listingID,
		ModelVersion:  "duckdb_baseline_v1",
		IsColdStart:   isColdStart,
		DailyForecast: points,
	}, nil
}

// PlatformOrderSummary counts distinct paid orders and sums GMV over order_facts
// for occurred_at >= since, across all sellers.
func (r *DuckDBRepository) PlatformOrderSummary(ctx context.Context, since time.Time) (OrderSummary, error) {
	q := fmt.Sprintf(`
SELECT
  COUNT(DISTINCT order_id)                AS orders,
  COALESCE(SUM(quantity * unit_price), 0) AS gmv
FROM %s
WHERE occurred_at >= ?`, warehouse.OrderFactsTableName)

	var s OrderSummary
	if err := r.db.QueryRowContext(ctx, q, since.UTC()).Scan(&s.OrderCount, &s.GMV); err != nil {
		return OrderSummary{}, fmt.Errorf("platform order summary query: %w", err)
	}
	return s, nil
}

// RecentOrders returns the latest paid orders, one row per order.
func (r *DuckDBRepository) RecentOrders(ctx context.Context, limit int) ([]RecentOrder, error) {
	q := fmt.Sprintf(`
SELECT
  order_id,
  MIN(seller_id)                AS seller_id,
  SUM(quantity * unit_price)    AS total,
  MAX(occurred_at)              AS paid_at
FROM %s
GROUP BY order_id
ORDER BY paid_at DESC, order_id ASC
LIMIT ?`, warehouse.OrderFactsTableName)

	rows, err := r.db.QueryContext(ctx, q, limit)
	if err != nil {
		return nil, fmt.Errorf("recent orders query: %w", err)
	}
	defer rows.Close()
	var out []RecentOrder
	for rows.Next() {
		var o RecentOrder
		if err := rows.Scan(&o.OrderID, &o.SellerID, &o.Total, &o.PaidAt); err != nil {
			return nil, fmt.Errorf("scan recent order: %w", err)
		}
		o.PaidAt = o.PaidAt.UTC()
		out = append(out, o)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate recent orders: %w", err)
	}
	return out, nil
}

// compile-time assertion that the adapter satisfies the seam.
var _ Repository = (*DuckDBRepository)(nil)

// listingScopedTypes are the event types that must carry a listing_id.
var listingScopedTypes = map[string]bool{"view": true, "click": true, "add_to_cart": true, "impression": true}

// TrackingQuality measures the tracking stream over [since, until] on occurred_at
// from tracking_events_resolved, plus the ingest counters of the overlapping hours.
func (r *DuckDBRepository) TrackingQuality(ctx context.Context, since, until time.Time) (TrackingQualityData, error) {
	var out TrackingQualityData
	since, until = since.UTC(), until.UTC()

	typeQ := fmt.Sprintf(`
SELECT event_type,
  COUNT(*)                          AS events,
  COUNT(DISTINCT user_key)          AS visitors,
  AVG(CASE WHEN COALESCE(listing_id, '') = '' THEN 1.0 ELSE 0.0 END) AS missing_ratio
FROM %s
WHERE occurred_at >= ? AND occurred_at <= ?
GROUP BY event_type
ORDER BY event_type`, warehouse.ResolvedViewName)
	rows, err := r.db.QueryContext(ctx, typeQ, since, until)
	if err != nil {
		return out, fmt.Errorf("tracking quality per-type query: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var t TypeQuality
		if err := rows.Scan(&t.EventType, &t.Events, &t.Visitors, &t.MissingListingRatio); err != nil {
			return out, fmt.Errorf("scan tracking type quality: %w", err)
		}
		t.ListingScoped = listingScopedTypes[t.EventType]
		if !t.ListingScoped {
			t.MissingListingRatio = 0
		}
		out.Types = append(out.Types, t)
	}
	if err := rows.Err(); err != nil {
		return out, fmt.Errorf("iterate tracking type quality: %w", err)
	}

	// Rows without ingested_at (written before tracking-ingest-integrity) are
	// excluded from the lag statistics and the freshness.
	lagQ := fmt.Sprintf(`
SELECT
  quantile_cont(epoch(ingested_at) - epoch(occurred_at), 0.5),
  quantile_cont(epoch(ingested_at) - epoch(occurred_at), 0.95),
  max(ingested_at)
FROM %s
WHERE occurred_at >= ? AND occurred_at <= ? AND ingested_at IS NOT NULL`, warehouse.TableName)
	var p50, p95 sql.NullFloat64
	var last sql.NullTime
	if err := r.db.QueryRowContext(ctx, lagQ, since, until).Scan(&p50, &p95, &last); err != nil {
		return out, fmt.Errorf("tracking quality lag query: %w", err)
	}
	if p50.Valid && p95.Valid {
		out.HasLag, out.LagP50Seconds, out.LagP95Seconds = true, p50.Float64, p95.Float64
	}
	if last.Valid {
		out.LastIngestedAt = last.Time.UTC()
	}

	counterQ := fmt.Sprintf(`
SELECT COALESCE(SUM(decode_failures), 0), COALESCE(SUM(duplicates_skipped), 0)
FROM %s
WHERE hour >= ? AND hour <= ?`, warehouse.CountersTableName)
	if err := r.db.QueryRowContext(ctx, counterQ, since.Truncate(time.Hour), until).
		Scan(&out.DecodeFailures, &out.Duplicates); err != nil {
		return out, fmt.Errorf("tracking quality counters query: %w", err)
	}
	return out, nil
}

var _ QualityRepository = (*DuckDBRepository)(nil)

// FallbackModelVersion is the model_version serving stamps on fallback lists.
const FallbackModelVersion = "serving-fallback"

// RecommendationPerformance attributes clicks, add-to-carts and purchases to the
// (placement, model_version) of the impression that was served, over
// tracking_events_resolved (recs-attribution-hardening). Purchases are PAID
// order_facts lines, not beacons. A conversion is credited once, to the earliest
// click by the same user_key on the same listing that precedes it by at most
// attributionHours. MatureClicks counts clicks whose attribution window had
// closed at until; MaturePurchases the purchases credited to them.
func (r *DuckDBRepository) RecommendationPerformance(ctx context.Context, since, until time.Time, attributionHours int) ([]PerformanceRow, error) {
	since, until = since.UTC(), until.UTC()
	q := fmt.Sprintf(`
WITH ie AS (
  SELECT event_id, impression_id, placement_id, COALESCE(model_version, '') AS model_version,
         listing_id, occurred_at
  FROM %[1]s
  WHERE event_type = 'impression' AND occurred_at >= ? AND occurred_at <= ?
    AND COALESCE(impression_id, '') <> '' AND COALESCE(placement_id, '') <> ''
),
-- An impression is the triple (impression_id, placement_id, model_version): a reused id
-- under another placement or model is another impression, not collapsed by min().
imp AS (
  SELECT impression_id, placement_id, model_version, count(*) AS item_impressions
  FROM ie GROUP BY impression_id, placement_id, model_version
),
-- A click counts only on a listing the impression showed, at or before the click, and is
-- credited to exactly one impression: the latest qualifying impression event. A click that
-- carries a placement or model must match it; an empty one matches any.
clk AS (
  SELECT e.event_id, e.user_key, e.listing_id, e.occurred_at, ie.placement_id, ie.model_version,
         (e.occurred_at + to_hours(CAST(? AS BIGINT)) <= ?) AS mature
  FROM %[1]s e JOIN ie
    ON e.impression_id = ie.impression_id AND e.listing_id = ie.listing_id
   AND ie.occurred_at <= e.occurred_at
  WHERE e.event_type = 'click' AND e.occurred_at >= ? AND e.occurred_at <= ?
    AND (COALESCE(e.placement_id, '') = '' OR e.placement_id = ie.placement_id)
    AND (COALESCE(e.model_version, '') = '' OR e.model_version = ie.model_version)
  QUALIFY row_number() OVER (PARTITION BY e.event_id
          ORDER BY ie.occurred_at DESC, ie.placement_id, ie.model_version, ie.event_id) = 1
),
-- Add-to-carts stay client beacons (no server-side cart fact in the warehouse).
atc AS (
  SELECT k.placement_id, k.model_version
  FROM %[1]s c JOIN clk k
    ON c.user_key = k.user_key AND c.listing_id = k.listing_id
   AND c.occurred_at BETWEEN k.occurred_at AND k.occurred_at + to_hours(CAST(? AS BIGINT))
  WHERE c.event_type = 'add_to_cart' AND c.occurred_at <= ?
    AND COALESCE(c.listing_id, '') <> ''
    -- An event with no user and no anonymous id resolves to the shared key 'anon:';
    -- joining on it would credit one visitor's cart to another visitor's click.
    AND c.user_key <> 'anon:'
  QUALIFY row_number() OVER (PARTITION BY c.event_id ORDER BY k.occurred_at, k.event_id) = 1
),
-- Purchases are server truth: PAID order lines whose buyer is the click's user. A purchase
-- beacon is ignored (forgeable); a line with no buyer is never attributed.
pur AS (
  SELECT k.placement_id, k.model_version, k.mature
  FROM %[2]s o JOIN clk k
    ON o.buyer_id = k.user_key AND o.listing_id = k.listing_id
   AND o.occurred_at BETWEEN k.occurred_at AND k.occurred_at + to_hours(CAST(? AS BIGINT))
  WHERE o.status = 'PAID' AND COALESCE(o.buyer_id, '') <> '' AND o.occurred_at <= ?
  QUALIFY row_number() OVER (PARTITION BY o.event_id ORDER BY k.occurred_at, k.event_id) = 1
),
i AS (
  SELECT placement_id, model_version, count(*) AS impressions, sum(item_impressions) AS item_impressions
  FROM imp GROUP BY placement_id, model_version
),
k AS (
  SELECT placement_id, model_version, count(*) AS clicks,
         count(*) FILTER (WHERE mature) AS mature_clicks
  FROM clk GROUP BY placement_id, model_version
),
v AS (
  SELECT placement_id, model_version, count(*) AS add_to_carts FROM atc GROUP BY placement_id, model_version
),
p AS (
  SELECT placement_id, model_version, count(*) AS purchases,
         count(*) FILTER (WHERE mature) AS mature_purchases
  FROM pur GROUP BY placement_id, model_version
)
SELECT i.placement_id, i.model_version, i.impressions, i.item_impressions,
       COALESCE(k.clicks, 0), COALESCE(v.add_to_carts, 0), COALESCE(p.purchases, 0),
       COALESCE(k.mature_clicks, 0), COALESCE(p.mature_purchases, 0)
FROM i
LEFT JOIN k USING (placement_id, model_version)
LEFT JOIN v USING (placement_id, model_version)
LEFT JOIN p USING (placement_id, model_version)
ORDER BY i.placement_id, i.model_version`, warehouse.ResolvedViewName, warehouse.OrderFactsTableName)
	rows, err := r.db.QueryContext(ctx, q,
		since, until, // ie
		attributionHours, until, since, until, // clk: mature flag, window
		attributionHours, until, // atc
		attributionHours, until) // pur
	if err != nil {
		return nil, fmt.Errorf("recommendation performance query: %w", err)
	}
	defer rows.Close()
	var out []PerformanceRow
	for rows.Next() {
		var p PerformanceRow
		if err := rows.Scan(&p.PlacementID, &p.ModelVersion, &p.Impressions, &p.ItemImpressions,
			&p.Clicks, &p.AddToCarts, &p.Purchases, &p.MatureClicks, &p.MaturePurchases); err != nil {
			return nil, fmt.Errorf("scan recommendation performance: %w", err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate recommendation performance: %w", err)
	}
	return out, nil
}

var _ PerformanceRepository = (*DuckDBRepository)(nil)
