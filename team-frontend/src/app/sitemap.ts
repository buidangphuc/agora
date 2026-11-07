import type { MetadataRoute } from "next";

import { listCategories } from "@/lib/gateway/listings";
import { readSitemapListings } from "@/lib/gateway/sitemap";
import { absoluteUrl } from "@/lib/seo";

export const revalidate = 3600;

export default async function sitemap(): Promise<MetadataRoute.Sitemap> {
  const [categories, listings] = await Promise.all([
    listCategories().catch(() => []),
    readSitemapListings(),
  ]);
  const shops = new Set<string>();
  for (const l of listings) if (l.sellerId) shops.add(l.sellerId);
  return [
    { url: absoluteUrl("/") },
    ...categories.map((c) => ({
      url: absoluteUrl(`/search?category=${encodeURIComponent(c.id)}`),
    })),
    ...listings.map((l) => ({ url: absoluteUrl(`/listing/${l.listingId}`) })),
    ...[...shops].map((s) => ({ url: absoluteUrl(`/shop/${s}`) })),
  ];
}
