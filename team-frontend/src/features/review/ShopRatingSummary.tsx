import { Rate } from "@/components/ui/Rate";
import type { ViewShopRatingSummary } from "@/lib/gateway/reviews";

/**
 * Compact shop rating rollup (average + count) sourced from
 * GetShopRatingSummary on team-engagement. Presentational: the caller
 * server-fetches and passes the summary in. A shop with no reviews shows
 * "Chưa có đánh giá" (never an invented 0.0 / 5.0 or 5.0 / 5.0).
 */
export function ShopRatingSummary({
  summary,
}: {
  summary: ViewShopRatingSummary;
}) {
  const rated = summary.reviewCount > 0;
  return (
    <div
      data-testid="shop-rating-summary"
      className="flex flex-wrap items-center gap-2 text-sm"
    >
      {rated ? (
        <>
          <Rate readOnly size="sm" value={summary.averageRating} />
          <strong className="text-text-primary">
            {summary.averageRating.toFixed(1)} / 5.0
          </strong>
          <span className="text-text-secondary">
            ({summary.reviewCount} đánh giá)
          </span>
        </>
      ) : (
        <span className="text-text-secondary">Chưa có đánh giá</span>
      )}
    </div>
  );
}
