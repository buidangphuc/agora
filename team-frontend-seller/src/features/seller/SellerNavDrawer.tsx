"use client";

import { usePathname } from "next/navigation";
import { useState } from "react";

import { Drawer } from "@/components/ui/Drawer";
import { SellerLogoutButton } from "./SellerLogoutButton";
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
    <div className="sticky top-0 z-30 flex min-h-16 items-center gap-3 border-b border-border-subtle bg-surface-card px-4 py-2 print:hidden md:hidden">
      <button
        type="button"
        onClick={() => setOpenedOn(pathname)}
        aria-label="Mở menu người bán"
        aria-haspopup="dialog"
        className="inline-flex h-11 w-11 shrink-0 items-center justify-center rounded-xl text-text-primary transition hover:bg-surface-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring"
      >
        <NavIcon name="menu" />
      </button>
      <div className="min-w-0 flex-1">
        <p className="text-xs font-medium text-text-secondary">
          Kênh người bán
        </p>
        <p className="truncate text-sm font-semibold text-text-primary">
          {shop.name}
        </p>
      </div>
      <Drawer
        isOpen={open}
        onClose={() => setOpenedOn(null)}
        title="Kênh người bán"
        placement="left"
        size="sm"
      >
        <div className="space-y-6">
          <ShopCard shop={shop} />
          <SellerNavList onNavigate={() => setOpenedOn(null)} />
          <SellerLogoutButton />
        </div>
      </Drawer>
    </div>
  );
}
