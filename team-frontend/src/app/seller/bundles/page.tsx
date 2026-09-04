import { redirect } from "next/navigation";

import { BundleManager } from "@/features/seller/BundleManager";
import {
  type ListingPage,
  listBundlesBySeller,
  listMyListings,
} from "@/lib/gateway/listings";
import { getPrincipal, hasScope } from "@/lib/gateway/session";

export const dynamic = "force-dynamic";

export const metadata = {
  title: "Combo sản phẩm | Kênh người bán",
};

export default async function SellerBundlesPage() {
  const principal = getPrincipal();
  if (!principal || !hasScope("listing.write")) redirect("/login");

  let page: ListingPage = { items: [], nextCursor: "", total: 0 };
  try {
    page = await listMyListings();
  } catch {
    // fall through with empty list
  }
  const bundles = await listBundlesBySeller(principal.id);

  const listings = page.items.map((l) => ({ id: l.id, title: l.title }));

  return (
    <div className="space-y-4">
      <div className="rounded-xs border border-gray-200 bg-white p-5 shadow-2xs">
        <h1 className="text-lg font-bold text-gray-900">🧩 Combo sản phẩm</h1>
        <p className="mt-0.5 text-xs text-gray-500">
          Gộp nhiều sản phẩm thành một combo với giá ưu đãi.
        </p>
      </div>

      <BundleManager listings={listings} initialBundles={bundles} />
    </div>
  );
}
