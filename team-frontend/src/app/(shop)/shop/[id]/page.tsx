import type { Metadata } from "next";

import { parseSort } from "@/features/listing/pdp";
import { BundleCard } from "@/features/shop/BundleCard";
import { ShopStorefrontView } from "@/features/shop/ShopStorefrontView";
import { isFollowing } from "@/lib/gateway/engagement";
import { getListing, listBundlesBySeller } from "@/lib/gateway/listings";
import { getShopRatingSummary } from "@/lib/gateway/reviews";
import { searchListings } from "@/lib/gateway/search";
import { getPrincipal } from "@/lib/gateway/session";
import { pageMetadata } from "@/lib/seo";

import { loadStorefront } from "./data";

export const dynamic = "force-dynamic";

export async function generateMetadata(props: {
  params: Promise<{ id: string }>;
}): Promise<Metadata> {
  const { id } = await props.params;
  const storefront = await loadStorefront(id).catch(() => null);
  const name = storefront?.displayName?.trim() || `Shop #${id.slice(0, 6)}`;
  return pageMetadata({
    title: name,
    description: storefront?.tagline?.trim() || `Cửa hàng ${name} trên Agora`,
    path: `/shop/${id}`,
    image: storefront?.bannerUrl || undefined,
  });
}

export default async function ShopPage(props: {
  params: Promise<{ id: string }>;
  searchParams: Promise<Record<string, string | string[] | undefined>>;
}) {
  const searchParams = await props.searchParams;
  const params = await props.params;
  const me = getPrincipal();
  const [shop, following, storefront, bundles, summary] = await Promise.all([
    // Only this seller's published listings: the search service filters by seller_id
    // (ListListings has no seller filter, and its first page holds other shops' items).
    searchListings("", { sellerId: params.id }).catch(() => ({ items: [] })),
    me ? isFollowing(params.id) : Promise.resolve(false),
    loadStorefront(params.id),
    listBundlesBySeller(params.id),
    getShopRatingSummary(params.id),
  ]);

  const listed = new Set(shop.items.map((l) => l.id));
  const missing = [...new Set(storefront?.featuredListingIds ?? [])].filter(
    (id) => !listed.has(id),
  );
  const fetchedFeatured = (
    await Promise.all(missing.map((id) => getListing(id).catch(() => null)))
  ).filter((l) => l !== null);

  return (
    <section className="py-2">
      <ShopStorefrontView
        sellerId={params.id}
        listings={shop.items}
        loggedIn={me !== null}
        initialFollowing={following}
        storefront={storefront}
        summary={summary}
        sort={parseSort(searchParams.sort)}
        fetchedFeatured={fetchedFeatured}
      />

      {/* ── Bundles / combos offered by this shop ── */}
      {bundles.length > 0 && (
        <div className="mt-6 rounded-xs border border-border-subtle bg-surface-card p-5 shadow-2xs">
          <h2 className="mb-3 text-sm font-bold text-text-primary">
            🧩 Combo tiết kiệm ({bundles.length})
          </h2>
          <ul className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
            {bundles.map((b) => (
              <BundleCard
                key={b.id}
                bundle={b}
                shopListings={shop.items}
                shopId={params.id}
              />
            ))}
          </ul>
        </div>
      )}
    </section>
  );
}
