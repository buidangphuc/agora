import { ShopStorefrontView } from "@/features/shop/ShopStorefrontView";
import { isFollowing } from "@/lib/gateway/engagement";
import {
  getStorefront,
  listBundlesBySeller,
  listListings,
} from "@/lib/gateway/listings";
import { getPrincipal } from "@/lib/gateway/session";

export const dynamic = "force-dynamic";

export default async function ShopPage({
  params,
}: {
  params: { id: string };
}) {
  const me = getPrincipal();
  const [page, following, storefront, bundles] = await Promise.all([
    listListings({ status: "published" }).catch(() => ({
      items: [],
      nextCursor: "",
      total: 0,
    })),
    me ? isFollowing(params.id) : Promise.resolve(false),
    getStorefront(params.id),
    listBundlesBySeller(params.id),
  ]);

  const shopListings = page.items.filter(
    (l) => l.sellerId === params.id || !l.sellerId,
  );

  return (
    <section className="py-2">
      <ShopStorefrontView
        sellerId={params.id}
        listings={shopListings.length > 0 ? shopListings : page.items}
        loggedIn={me !== null}
        initialFollowing={following}
        storefront={storefront}
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
