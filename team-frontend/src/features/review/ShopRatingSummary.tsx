import type { ViewShopRatingSummary } from "@/lib/gateway/reviews";

/**
 * Compact shop rating rollup (average + count) sourced from
 * GetShopRatingSummary on team-engagement. Presentational — the caller
 * server-fetches and passes the summary in.
 */
export function ShopRatingSummary({
  summary,
}: {
  summary: ViewShopRatingSummary;
}) {
  return (
    <div data-testid="shop-rating-summary" className="flex items-center gap-2">
      <span className="text-yellow-400 text-sm">★</span>
      <strong className="text-brand">
        {summary.averageRating.toFixed(1)} / 5.0
      </strong>
      <span className="text-gray-400">({summary.reviewCount} đánh giá)</span>
    </div>
  );
}
