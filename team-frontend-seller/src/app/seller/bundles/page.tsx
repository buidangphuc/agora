import { redirectGuestToLogin } from "@/features/auth/guardPath";

import { BundleManager } from "@/features/seller/BundleManager";
import { SellerPageHeader } from "@/features/seller/SellerPageHeader";
import { listBundlesBySeller } from "@/lib/gateway/listings";
import { getPrincipal, hasScope } from "@/lib/gateway/session";

export const dynamic = "force-dynamic";

export const metadata = { title: "Combo sản phẩm" };

export default async function SellerBundlesPage() {
  const principal = getPrincipal();
  if (!principal) redirectGuestToLogin("/seller/bundles");
  if (!hasScope("listing.write")) return null;

  const bundles = await listBundlesBySeller(principal.id);

  return (
    <>
      <SellerPageHeader
        title="Combo sản phẩm"
        description="Gộp nhiều sản phẩm thành một combo với giá ưu đãi."
      />
      <BundleManager bundles={bundles} />
    </>
  );
}
