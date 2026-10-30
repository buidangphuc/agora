"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";

import { NavIcon, SELLER_NAV, isNavActive } from "./nav";

/**
 * The seller links. Shared by the desktop sidebar and the mobile Drawer.
 * The active route carries aria-current="page"; when `collapsed` the label is
 * visually hidden but stays the link's accessible name (plus a title tooltip).
 */
export function SellerNavList({
  collapsed = false,
  onNavigate,
}: { collapsed?: boolean; onNavigate?: () => void }) {
  const pathname = usePathname() ?? "";
  return (
    <nav aria-label="Kênh người bán">
      <ul className="space-y-1">
        {SELLER_NAV.map((item) => {
          const active = isNavActive(item.href, pathname);
          return (
            <li key={item.href}>
              <Link
                href={item.href}
                title={collapsed ? item.label : undefined}
                aria-current={active ? "page" : undefined}
                onClick={onNavigate}
                className={`flex items-center gap-3 rounded-lg px-3 py-2 text-sm font-medium transition duration-150 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring ${
                  collapsed ? "justify-center" : ""
                } ${
                  active
                    ? "bg-primary-50 text-action-primary"
                    : "text-text-secondary hover:bg-surface-page hover:text-text-primary"
                }`}
              >
                <NavIcon name={item.icon} />
                <span className={collapsed ? "sr-only" : ""}>{item.label}</span>
              </Link>
            </li>
          );
        })}
      </ul>
    </nav>
  );
}
