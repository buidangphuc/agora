import Link from "next/link";
import { redirect } from "next/navigation";

import { SellerPageHeader } from "@/features/seller/SellerPageHeader";
import { ShopProfileForm } from "@/features/seller/ShopProfileForm";
import { getStorefront } from "@/lib/gateway/listings";
import { getPrincipal, hasScope } from "@/lib/gateway/session";

export const dynamic = "force-dynamic";

export const metadata = { title: "Hồ sơ gian hàng | Kênh người bán" };

/** Form > Basic Form: the shop display name (UpsertStorefront.display_name). */
export default async function SellerShopPage() {
  const me = getPrincipal();
  if (!me || !hasScope("listing.write")) redirect("/login");

  const storefront = await getStorefront(me.id);

  return (
    <>
      <SellerPageHeader
        title="Hồ sơ gian hàng"
        description="Tên gian hàng người mua nhìn thấy trên trang shop và trang sản phẩm."
        action={
          <Link
            href={`/shop/${me.id}`}
            className="text-sm font-medium text-action-primary hover:underline"
          >
            Xem gian hàng công khai
          </Link>
        }
      />
      <ShopProfileForm initialName={storefront?.displayName ?? ""} />
    </>
  );
}
