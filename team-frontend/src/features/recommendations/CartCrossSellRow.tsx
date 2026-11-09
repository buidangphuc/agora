import { ListingGrid } from "@/features/listing/ListingGrid";
import {
  RecommendationContext,
  getRecommendations,
} from "@/lib/gateway/recommendations";

const MAX_LINES = 50;
const LIMIT = 8;

export async function CartCrossSellRow({
  lines,
}: {
  lines: { listingId: string }[];
}) {
  if (lines.length === 0) return null;

  const contextListingIds = [
    ...new Set(lines.slice(-MAX_LINES).map((l) => l.listingId)),
  ];

  const recs = await getRecommendations({
    context: RecommendationContext.CART,
    limit: LIMIT,
    contextListingIds,
  }).catch(() => null);

  if (!recs || recs.items.length === 0) return null;

  const { requestId, modelVersion } = recs;
  const placementId = recs.placementId || "cart_cross_sell";

  return (
    <section className="space-y-3" data-recs-request-id={requestId}>
      <div className="rounded-xs bg-surface-card p-3 shadow-card border-b-2 border-action-primary flex items-center justify-between">
        <span className="font-bold text-sm text-action-primary uppercase tracking-wider flex items-center gap-1.5">
          <span>✨</span>
          <span>Gợi ý cho bạn</span>
        </span>
        <span className="text-xs text-text-disabled font-normal">
          Thường được mua cùng
        </span>
      </div>
      <ListingGrid
        listings={recs.items.slice(0, LIMIT)}
        placementId={placementId}
        impressionId={requestId}
        modelVersion={modelVersion}
      />
    </section>
  );
}
