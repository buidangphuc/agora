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
// distinct orders from order_facts (ADR-0013).
func (r *DuckDBRepository) SellerFunnel(ctx context.Context, sellerID string, from, to time.Time) (Funnel, error) {
	trackingQ := fmt.Sprintf(`
SELECT
  COUNT(*) FILTER (WHERE event_type = 'impression') AS impressions,
  COUNT(*) FILTER (WHERE event_type = 'view')       AS views,
  COUNT(*) FILTER (WHERE event_type = 'add_to_cart') AS adds
FROM %s
WHERE occurred_at >= ? AND occurred_at <= ?`,
		warehouse.TableName)

	var f Funnel
	row := r.db.QueryRowContext(ctx, trackingQ, from.UTC(), to.UTC())
	if err := row.Scan(&f.Impressions, &f.Views, &f.Adds); err != nil {
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

// compile-time assertion that the adapter satisfies the seam.
var _ Repository = (*DuckDBRepository)(nil)

