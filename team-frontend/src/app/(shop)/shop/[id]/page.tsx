import { parseSort } from "@/features/listing/pdp";
import { ShopStorefrontView } from "@/features/shop/ShopStorefrontView";
import { isFollowing } from "@/lib/gateway/engagement";
import { getStorefront, listBundlesBySeller } from "@/lib/gateway/listings";
import { getShopRatingSummary } from "@/lib/gateway/reviews";
import { searchListings } from "@/lib/gateway/search";
import { getPrincipal } from "@/lib/gateway/session";

export const dynamic = "force-dynamic";

export default async function ShopPage({
  params,
  searchParams,
}: {
  params: { id: string };
  searchParams: Record<string, string | string[] | undefined>;
}) {
  const me = getPrincipal();
  const [shop, following, storefront, bundles, summary] = await Promise.all([
    // Only this seller's published listings: the search service filters by seller_id
    // (ListListings has no seller filter, and its first page holds other shops' items).
    searchListings("", { sellerId: params.id }).catch(() => ({ items: [] })),
    me ? isFollowing(params.id) : Promise.resolve(false),
    getStorefront(params.id),
    listBundlesBySeller(params.id),
    getShopRatingSummary(params.id),
  ]);

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
      />

      {/* ── Bundles / combos offered by this shop ── */}
      {bundles.length > 0 && (
        <div className="mt-6 rounded-xs border border-gray-200 bg-white p-5 shadow-2xs">
          <h2 className="mb-3 text-sm font-bold text-gray-800">
            🧩 Combo tiết kiệm ({bundles.length})
          </h2>
          <ul className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
            {bundles.map((b) => (
              <li
                key={b.id}
                className="rounded-xs border border-gray-100 p-3 text-xs"
              >
                <p className="font-semibold text-gray-800">{b.title}</p>
                <p className="mt-0.5 text-gray-400">
                  {b.listingIds.length} sản phẩm
                </p>
                <p className="mt-1 font-bold text-brand">
                  {b.bundlePrice.toLocaleString("vi-VN")} VND
                </p>
              </li>
            ))}
          </ul>
        </div>
      )}
    </section>
  );
}
