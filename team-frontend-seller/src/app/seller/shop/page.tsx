import { Link } from "@/components/ui/Link";
import { redirectGuestToLogin } from "@/features/auth/guardPath";

import { SellerPageHeader } from "@/features/seller/SellerPageHeader";
import { ShopProfileForm } from "@/features/seller/ShopProfileForm";
import { buyerAppUrl } from "@/lib/apps";
import { getStorefront } from "@/lib/gateway/listings";
import { getPrincipal, hasScope } from "@/lib/gateway/session";

export const dynamic = "force-dynamic";

export const metadata = { title: "Hồ sơ gian hàng | Kênh người bán" };

/** Form > Basic Form: the shop display name (UpsertStorefront.display_name). */
export default async function SellerShopPage() {
  const me = getPrincipal();
  if (!me) redirectGuestToLogin("/seller/shop");
  if (!hasScope("listing.write")) return null;

  const storefront = await getStorefront(me.id);

  return (
    <>
      <SellerPageHeader
        title="Hồ sơ gian hàng"
        description="Tên gian hàng người mua nhìn thấy trên trang shop và trang sản phẩm."
        action={
          <span className="text-sm font-medium">
            <Link href={buyerAppUrl(`/shop/${me.id}`)}>
              Xem gian hàng công khai
            </Link>
          </span>
        }
      />
      <ShopProfileForm initialName={storefront?.displayName ?? ""} />
    </>
  );
}
