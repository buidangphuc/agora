import { ListingGrid } from "@/features/listing/ListingGrid";
import { getRecentlyViewed } from "@/lib/gateway/engagement";
import { type ViewListing, getListing } from "@/lib/gateway/listings";

/**
 * "Vừa xem" — recently-viewed listings for the signed-in user, sourced from
 * team-engagement (GetRecentlyViewed) via the gateway (Rule 1). The RPC returns
 * listing ids only (engagement never owns listing content, Rule 3), so each id
 * is hydrated into a card through the existing listing client — the same path
 * search/recommendations use.
 *
 * Renders nothing when there is no history (anonymous user or service
 * unavailable), so the home page degrades gracefully.
 */
export async function RecentlyViewedRow({ limit = 12 }: { limit?: number }) {
  const ids = await getRecentlyViewed(limit).catch(() => []);
  if (ids.length === 0) return null;

  const resolved = await Promise.all(ids.map((id) => getListing(id)));
  const items = resolved.filter((l): l is ViewListing => l !== null);
  if (items.length === 0) return null;

  return (
    <section className="space-y-3">
      <div className="rounded-xs bg-white p-3 shadow-shopee border-b-2 border-brand flex items-center justify-between">
        <span className="font-bold text-sm text-brand uppercase tracking-wider flex items-center gap-1.5">
          <span>👀</span>
          <span>Vừa xem</span>
        </span>
        <span className="text-[12px] text-gray-400 font-normal">
          Sản phẩm bạn đã xem gần đây
        </span>
      </div>
      <ListingGrid listings={items} />
    </section>
  );
}
