import type { ViewListing, ViewStorefront } from "@/lib/gateway/listings";

export const MAX_FEATURED = 6;

export function resolveFeatured(
  storefront: ViewStorefront | null,
  shopListings: ViewListing[],
  fetched: (ViewListing | null)[],
): ViewListing[] {
  if (!storefront) return [];
  const pool = new Map<string, ViewListing>();
  for (const l of [...fetched, ...shopListings]) {
    if (l) pool.set(l.id, l);
  }
  const seen = new Set<string>();
  const out: ViewListing[] = [];
  for (const id of storefront.featuredListingIds) {
    if (out.length >= MAX_FEATURED) break;
    if (seen.has(id)) continue;
    seen.add(id);
    const l = pool.get(id);
    if (!l) continue;
    if (l.sellerId !== storefront.sellerId) continue;
    if (l.status && l.status !== "published") continue;
    out.push(l);
  }
  return out;
}
