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
 * unavailable). A failed id lookup (e.g. team-domain down) is treated like a
 * missing listing, never thrown: the block degrades locally and the rest of
 * the home page still renders.
 */
export async function RecentlyViewedRow({ limit = 12 }: { limit?: number }) {
  const ids = await getRecentlyViewed(limit).catch(() => []);
  if (ids.length === 0) return null;

  const resolved = await Promise.all(
    ids.map((id) => getListing(id).catch(() => null)),
  );
  const items = resolved.filter((l): l is ViewListing => l !== null);
  if (items.length === 0) return null;

  return (
    <section aria-labelledby="recently-viewed-title" className="space-y-4">
      <h2
        id="recently-viewed-title"
        className="flex min-h-16 items-center justify-between gap-3 rounded-2xl border border-border-subtle bg-surface-card px-5 py-4 text-lg font-semibold text-text-primary shadow-preline-card"
      >
        Vừa xem
        <span className="text-xs font-normal text-text-secondary">
          Sản phẩm bạn đã xem gần đây
        </span>
      </h2>
      <ListingGrid listings={items} />
    </section>
  );
}
