"use client";

import { useEffect, useState } from "react";

import { SellerLogoutButton } from "./SellerLogoutButton";
import { SellerNavList } from "./SellerNavList";
import { type SellerShopInfo, ShopCard } from "./ShopCard";
import { NavIcon } from "./nav";

export const SIDEBAR_STORAGE_KEY = "seller.sidebar.collapsed";

function readPreference(): boolean | null {
  try {
    const v = window.localStorage.getItem(SIDEBAR_STORAGE_KEY);
    return v === "1" ? true : v === "0" ? false : null;
  } catch {
    return null;
  }
}

function writePreference(collapsed: boolean) {
  try {
    window.localStorage.setItem(SIDEBAR_STORAGE_KEY, collapsed ? "1" : "0");
  } catch {
    // storage blocked: the preference just does not persist
  }
}

/**
 * Desktop sidebar (>= 768px; below that the Drawer is used). 256px expanded,
 * 64px collapsed. Server output is always the expanded state; the stored
 * preference (or "collapsed by default on tablet") applies after hydration.
 */
export function SellerSidebar({ shop }: { shop: SellerShopInfo }) {
  const [collapsed, setCollapsed] = useState(false);

  useEffect(() => {
    const stored = readPreference();
    if (stored !== null) setCollapsed(stored);
    else if (window.innerWidth < 1024) setCollapsed(true);
  }, []);

  function toggle() {
    setCollapsed((prev) => {
      writePreference(!prev);
      return !prev;
    });
  }

  return (
    <aside
      data-collapsed={collapsed ? "true" : "false"}
      className={`hidden shrink-0 border-r border-border-subtle bg-surface-card transition-all duration-200 print:hidden md:block ${
        collapsed ? "w-16" : "w-64"
      }`}
    >
      <div
        className={`sticky top-0 flex max-h-screen flex-col gap-6 overflow-y-auto py-6 ${
          collapsed ? "px-2" : "px-4"
        }`}
      >
        <ShopCard shop={shop} compact={collapsed} />
        <SellerNavList collapsed={collapsed} />
        <div className="space-y-2 border-t border-border-subtle pt-4">
          <button
            type="button"
            onClick={toggle}
            aria-expanded={!collapsed}
            aria-label={collapsed ? "Mở rộng thanh bên" : "Thu gọn thanh bên"}
            className="flex min-h-touch w-full items-center justify-center gap-2 rounded-xl px-3 text-xs font-medium text-text-secondary transition hover:bg-surface-muted hover:text-text-primary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring"
          >
            <NavIcon
              name={collapsed ? "chevron-right" : "chevron-left"}
              className="h-4 w-4"
            />
            {!collapsed && <span>Thu gọn</span>}
          </button>
          <SellerLogoutButton compact={collapsed} />
        </div>
      </div>
    </aside>
  );
}
