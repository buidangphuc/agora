import { ListingGrid } from "@/features/listing/ListingGrid";
import {
  RecommendationContext,
  getRecommendations,
} from "@/lib/gateway/recommendations";

/**
 * "Gợi ý cho bạn" — a recommendations row populated from team-ai via the
 * gateway (Rule 1). Server-only: the browser never calls team-ai directly.
 *
 * - Home ("for you"): no seed, context HOMEPAGE.
 * - PDP ("similar items"): seeded with the current listing id, context
 *   SIMILAR_ITEMS.
 *
 * Renders nothing when the list is empty (service unavailable or no results),
 * so the page still renders cleanly when RECS_ENABLED=false (Rule: graceful
 * degradation).
 */
export async function RecommendationsRow({
  seedListingId,
  limit = 10,
}: {
  seedListingId?: string;
  limit?: number;
}) {
  const items = await getRecommendations({
    seedListingId,
    context: seedListingId
      ? RecommendationContext.SIMILAR_ITEMS
      : RecommendationContext.HOMEPAGE,
    limit,
  }).catch(() => []);

  if (items.length === 0) return null;

  const heading = seedListingId ? "Sản phẩm tương tự" : "Gợi ý cho bạn";

  return (
    <section className="space-y-3">
      <div className="rounded-xs bg-white p-3 shadow-shopee border-b-2 border-brand flex items-center justify-between">
        <span className="font-bold text-sm text-brand uppercase tracking-wider flex items-center gap-1.5">
          <span>✨</span>
          <span>Gợi ý cho bạn</span>
        </span>
        <span className="text-[12px] text-gray-400 font-normal">{heading}</span>
      </div>
      <ListingGrid listings={items} />
    </section>
  );
}
