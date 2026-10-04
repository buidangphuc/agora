"use client";

import { usePathname } from "next/navigation";
import { useState } from "react";

import { Drawer } from "@/components/ui/Drawer";
import { SellerNavList } from "./SellerNavList";
import { type SellerShopInfo, ShopCard } from "./ShopCard";
import { NavIcon } from "./nav";

/**
 * Mobile (< 768px) top bar: a menu button that opens the seller navigation in a
 * Drawer. The Drawer closes on navigation (link click or route change) and on
 * Escape, and focus returns to the menu button (Drawer / useDialog).
 */
export function SellerNavDrawer({ shop }: { shop: SellerShopInfo }) {
  const pathname = usePathname();
  // The Drawer is open for the route it was opened on, so any navigation
  // closes it without an effect.
  const [openedOn, setOpenedOn] = useState<string | null>(null);
  const open = openedOn !== null && openedOn === pathname;

  return (
    <div className="flex items-center gap-3 border-b border-border-subtle bg-surface-card px-4 py-2 print:hidden md:hidden">
      <button
        type="button"
        onClick={() => setOpenedOn(pathname)}
        aria-label="Mở menu người bán"
        aria-haspopup="dialog"
        className="rounded-lg p-2 text-text-primary transition hover:bg-surface-page focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring"
      >
        <NavIcon name="menu" />
      </button>
      <span className="truncate text-sm font-semibold text-text-primary">
        {shop.name}
      </span>
      <Drawer
        isOpen={open}
        onClose={() => setOpenedOn(null)}
        title="Kênh người bán"
        placement="left"
        size="sm"
      >
        <div className="space-y-4">
          <ShopCard shop={shop} />
          <SellerNavList onNavigate={() => setOpenedOn(null)} />
        </div>
      </Drawer>
    </div>
  );
}
