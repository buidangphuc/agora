import { redirect } from "next/navigation";
import type { ReactNode } from "react";

import { redirectGuestToLoginFromHeader } from "@/features/auth/guardPath";
import { SellerNavDrawer } from "@/features/seller/SellerNavDrawer";
import { SellerSidebar } from "@/features/seller/SellerSidebar";
import { getPrincipal, hasSellerAccess } from "@/lib/gateway/session";
import { batchGetShopNames, shopLabel } from "@/lib/gateway/shops";
import { sellerSession } from "@/lib/session-config";

export default async function SellerLayout({
  children,
}: { children: ReactNode }) {
  const principal = getPrincipal();
  if (!principal) {
    return redirectGuestToLoginFromHeader();
  }

  if (!hasSellerAccess(principal)) {
    return redirect(sellerSession.forbiddenPath);
  }

  const names = await batchGetShopNames([principal.id]);
  const shop = {
    sellerId: principal.id,
    name: shopLabel(principal.id, names.get(principal.id)),
  };

  return (
    <div className="min-h-screen bg-surface-muted md:flex">
      <SellerSidebar shop={shop} />
      <div className="min-w-0 flex-1">
        <SellerNavDrawer shop={shop} />
        <div className="mx-auto max-w-6xl space-y-6 p-4 lg:p-6">{children}</div>
      </div>
    </div>
  );
}
