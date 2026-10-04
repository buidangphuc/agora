import type { ReactNode } from "react";

import { Result } from "@/components/ui/Result";
import { LinkButton } from "@/features/seller/LinkButton";
import { SellerNavDrawer } from "@/features/seller/SellerNavDrawer";
import { SellerSidebar } from "@/features/seller/SellerSidebar";
import { getPrincipal } from "@/lib/gateway/session";
import { batchGetShopNames, shopLabel } from "@/lib/gateway/shops";
import { redirect } from "next/navigation";

export default async function SellerLayout({
  children,
}: { children: ReactNode }) {
  const principal = getPrincipal();
  if (!principal) {
    redirect("/login");
    return null;
  }

  if (!principal.scopes.includes("listing.write")) {
    return (
      <Result
        status="error"
        title="Cần tài khoản Người Bán"
        subTitle="Tài khoản của bạn chưa có quyền Người Bán (listing.write). Hãy kích hoạt kênh người bán để đăng bán sản phẩm."
        extra={<LinkButton href="/">Quay lại trang chủ</LinkButton>}
      />
    );
  }

  // The real shop name (shop-display-name); "Shop #xxxxxx" only when unset.
  const names = await batchGetShopNames([principal.id]);
  const shop = {
    sellerId: principal.id,
    name: shopLabel(principal.id, names.get(principal.id)),
  };

  return (
    <div className="-mx-4 -my-5 min-h-viewport-main bg-surface-muted md:flex">
      <SellerSidebar shop={shop} />
      <div className="min-w-0 flex-1">
        <SellerNavDrawer shop={shop} />
        <div className="mx-auto max-w-6xl space-y-6 p-4 lg:p-6">{children}</div>
      </div>
    </div>
  );
}
