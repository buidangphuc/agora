import Link from "next/link";
import { redirect } from "next/navigation";

import { listFollowedSellers } from "@/lib/gateway/engagement";
import { searchListings } from "@/lib/gateway/search";
import { getPrincipal } from "@/lib/gateway/session";

export const dynamic = "force-dynamic";

export const metadata = {
  title: "Shop đang theo dõi | Marketplace",
};

export default async function FollowingPage() {
  if (!getPrincipal()) redirect("/login");

  const sellerIds = await listFollowedSellers();

  // Feed = published listings from the sellers the user follows. team-engagement's
  // ListFollowedListings depends on a listing-event consumer that isn't wired yet,
  // so we resolve the feed via the search service (filter by seller), which is live.
  const perSeller = await Promise.all(
    sellerIds.map((sid) =>
      searchListings("", { sellerId: sid })
        .then((r) => r.items)
        .catch(() => []),
    ),
  );
  const seen = new Set<string>();
  const listings = perSeller
    .flat()
    .filter((l) => {
      if (seen.has(l.id)) return false;
      seen.add(l.id);
      return true;
    })
    .slice(0, 24)
    .map((l) => ({ id: l.id, title: l.title }));

  return (
    <section className="space-y-6 py-2">
      <div className="rounded-2xl border border-gray-100 bg-white p-5 shadow-2xs">
        <h1 className="text-lg font-bold text-gray-900">🏪 Shop đang theo dõi</h1>
        <p className="mt-0.5 text-xs text-gray-500">
          Các gian hàng và sản phẩm bạn đang theo dõi.
        </p>
      </div>

      <div className="rounded-2xl border border-gray-100 bg-white p-5 shadow-2xs">
        <h2 className="mb-3 text-sm font-bold text-gray-800">
          Gian hàng ({sellerIds.length})
        </h2>
        {sellerIds.length === 0 ? (
          <p className="text-xs text-gray-400">
            Bạn chưa theo dõi gian hàng nào.
          </p>
        ) : (
          <ul className="flex flex-wrap gap-2">
            {sellerIds.map((id) => (
              <li key={id}>
                <Link
                  href={`/shop/${id}`}
                  className="inline-flex items-center gap-1.5 rounded-lg border border-gray-200 px-3 py-1.5 text-xs font-medium text-gray-700 hover:border-brand hover:text-brand"
                >
                  🏪 Shop #{id.slice(0, 6)}
                </Link>
              </li>
            ))}
          </ul>
        )}
      </div>

      <div className="rounded-2xl border border-gray-100 bg-white p-5 shadow-2xs">
        <h2 className="mb-3 text-sm font-bold text-gray-800">
          Sản phẩm ({listings.length})
        </h2>
        {listings.length === 0 ? (
          <p className="text-xs text-gray-400">
            Bạn chưa theo dõi sản phẩm nào.
          </p>
        ) : (
          <ul className="divide-y divide-gray-100">
            {listings.map((l) => (
              <li key={l.id} className="py-2.5">
                <Link
                  href={`/listing/${l.id}`}
                  className="text-sm font-medium text-gray-800 hover:text-brand"
                >
                  {l.title}
                </Link>
              </li>
            ))}
          </ul>
        )}
      </div>
    </section>
  );
}
