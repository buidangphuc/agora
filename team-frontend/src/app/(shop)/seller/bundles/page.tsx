import { redirect } from "next/navigation";

import { BundleManager } from "@/features/seller/BundleManager";
import { SellerPageHeader } from "@/features/seller/SellerPageHeader";
import { getAllListings } from "@/features/seller/listingsPage";
import { listBundlesBySeller } from "@/lib/gateway/listings";
import { getPrincipal, hasScope } from "@/lib/gateway/session";

export const dynamic = "force-dynamic";

export const metadata = { title: "Combo sản phẩm | Kênh người bán" };

export default async function SellerBundlesPage() {
  const principal = getPrincipal();
  if (!principal || !hasScope("listing.write")) redirect("/login");

  const [listings, bundles] = await Promise.all([
    getAllListings().catch(() => []),
    listBundlesBySeller(principal.id),
  ]);

  return (
    <>
      <SellerPageHeader
        title="Combo sản phẩm"
        description="Gộp nhiều sản phẩm thành một combo với giá ưu đãi."
      />
      <BundleManager
        listings={listings.map((l) => ({ id: l.id, title: l.title }))}
        bundles={bundles}
      />
    </>
  );
}
