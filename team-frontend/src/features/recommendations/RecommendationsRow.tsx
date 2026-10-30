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
 * so the page still renders cleanly when recommendations are unavailable (Rule: graceful
 * degradation).
 */
export async function RecommendationsRow({
  seedListingId,
  limit = 10,
}: {
  seedListingId?: string;
  limit?: number;
}) {
  const recs = await getRecommendations({
    seedListingId,
    context: seedListingId
      ? RecommendationContext.SIMILAR_ITEMS
      : RecommendationContext.HOMEPAGE,
    limit,
  }).catch(() => null);

  if (!recs || recs.items.length === 0) return null;

  const { items, requestId, modelVersion } = recs;
  // Prefer the placement the server actually served; fall back to the local
  // name only when the server did not report one.
  const placementId =
    recs.placementId || (seedListingId ? "similar_items" : "home_feed");

  const heading = seedListingId ? "Sản phẩm tương tự" : "Dành riêng cho bạn";

  return (
    <section className="space-y-3" data-recs-request-id={requestId}>
      <div className="rounded-xs bg-surface-card p-3 shadow-card border-b-2 border-action-primary flex items-center justify-between">
        <span className="font-bold text-sm text-action-primary uppercase tracking-wider flex items-center gap-1.5">
          <span>✨</span>
          <span>Gợi ý cho bạn</span>
        </span>
        <span className="text-xs text-text-disabled font-normal">
          {heading}
        </span>
      </div>
      <ListingGrid
        listings={items}
        placementId={placementId}
        impressionId={requestId}
        modelVersion={modelVersion}
      />
    </section>
  );
}
