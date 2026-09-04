package query

import (
	"context"
	"database/sql"
	"fmt"
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

// json_extract_string(properties, '$.key') pulls a value out of the properties
// JSON bag; the fields the seller dashboard needs are not first-class columns
// (see repository.go schema-adaptation note).
const (
	colSellerID = "json_extract_string(properties, '$.seller_id')"
	colOrderID  = "json_extract_string(properties, '$.order_id')"
	colRevenue  = "TRY_CAST(json_extract_string(properties, '$.revenue') AS BIGINT)"
	colSKU      = "json_extract_string(properties, '$.sku')"
	colUnits    = "TRY_CAST(json_extract_string(properties, '$.units') AS BIGINT)"
)

// SellerFunnel counts impressions/views/adds by event_type and orders as the
// count of distinct non-empty order_ids, all scoped to sellerID and [from, to].
func (r *DuckDBRepository) SellerFunnel(ctx context.Context, sellerID string, from, to time.Time) (Funnel, error) {
	q := fmt.Sprintf(`
SELECT
  COUNT(*) FILTER (WHERE event_type = 'impression') AS impressions,
  COUNT(*) FILTER (WHERE event_type = 'view')       AS views,
  COUNT(*) FILTER (WHERE event_type = 'add_to_cart') AS adds,
  COUNT(DISTINCT %[2]s) FILTER (WHERE %[2]s IS NOT NULL AND %[2]s <> '') AS orders
FROM %[1]s
WHERE %[3]s = ?
  AND occurred_at >= ? AND occurred_at <= ?`,
		warehouse.TableName, colOrderID, colSellerID)

	var f Funnel
	row := r.db.QueryRowContext(ctx, q, sellerID, from.UTC(), to.UTC())
	if err := row.Scan(&f.Impressions, &f.Views, &f.Adds, &f.Orders); err != nil {
		return Funnel{}, fmt.Errorf("seller funnel query: %w", err)
	}
	return f, nil
}

// RevenueBreakdown returns per-day revenue/order-count and the top-N SKUs by
// revenue for sellerID over [from, to].
func (r *DuckDBRepository) RevenueBreakdown(ctx context.Context, sellerID string, from, to time.Time, topN int) (Breakdown, error) {
	if topN <= 0 {
		topN = defaultTopSKULimit
	}
	var b Breakdown

	dayQ := fmt.Sprintf(`
SELECT
  strftime(occurred_at, '%%Y-%%m-%%d') AS day,
  COALESCE(SUM(%[2]s), 0)              AS revenue,
  COUNT(DISTINCT %[3]s)                AS order_count
FROM %[1]s
WHERE %[4]s = ?
  AND %[3]s IS NOT NULL AND %[3]s <> ''
  AND occurred_at >= ? AND occurred_at <= ?
GROUP BY 1
ORDER BY 1`,
		warehouse.TableName, colRevenue, colOrderID, colSellerID)

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
  %[2]s                   AS sku,
  ANY_VALUE(listing_id)   AS listing_id,
  COALESCE(SUM(%[3]s), 0)  AS revenue,
  COALESCE(SUM(%[4]s), 0)  AS units_sold
FROM %[1]s
WHERE %[5]s = ?
  AND %[2]s IS NOT NULL AND %[2]s <> ''
  AND occurred_at >= ? AND occurred_at <= ?
GROUP BY 1
ORDER BY revenue DESC, sku ASC
LIMIT ?`,
		warehouse.TableName, colSKU, colRevenue, colUnits, colSellerID)

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

// compile-time assertion that the adapter satisfies the seam.
var _ Repository = (*DuckDBRepository)(nil)
