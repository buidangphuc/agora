import type { ReactNode } from "react";

export interface SellerNavItem {
  href: string;
  label: string;
  icon: IconName;
}

export type IconName =
  | "package"
  | "plus"
  | "layers"
  | "megaphone"
  | "orders"
  | "chart"
  | "wallet"
  | "star"
  | "store"
  | "chat"
  | "menu"
  | "chevron-left"
  | "chevron-right";

/** The seller cockpit navigation, in display order. */
export const SELLER_NAV: SellerNavItem[] = [
  { href: "/seller", label: "Tất cả sản phẩm", icon: "package" },
  { href: "/seller/new", label: "Thêm sản phẩm mới", icon: "plus" },
  { href: "/seller/orders", label: "Quản lý đơn hàng", icon: "orders" },
  { href: "/seller/bundles", label: "Combo sản phẩm", icon: "layers" },
  { href: "/seller/ads", label: "Quảng cáo", icon: "megaphone" },
  { href: "/seller/analytics", label: "Báo cáo doanh thu", icon: "chart" },
  { href: "/seller/wallet", label: "Ví người bán", icon: "wallet" },
  { href: "/seller/plans", label: "Gói đăng ký", icon: "star" },
  { href: "/seller/shop", label: "Hồ sơ gian hàng", icon: "store" },
  { href: "/chat", label: "Chăm sóc khách hàng", icon: "chat" },
];

const PATHS: Record<IconName, string> = {
  package:
    "M16.5 9.4l-9-5.19M21 16V8a2 2 0 00-1-1.73l-7-4a2 2 0 00-2 0l-7 4A2 2 0 003 8v8a2 2 0 001 1.73l7 4a2 2 0 002 0l7-4A2 2 0 0021 16zM3.27 6.96L12 12.01l8.73-5.05M12 22.08V12",
  plus: "M12 5v14M5 12h14",
  layers: "M12 2L2 7l10 5 10-5-10-5zM2 17l10 5 10-5M2 12l10 5 10-5",
  megaphone:
    "M11 5L6 9H2v6h4l5 4V5zM19.07 4.93a10 10 0 010 14.14M15.54 8.46a5 5 0 010 7.07",
  orders:
    "M16 4h2a2 2 0 012 2v14a2 2 0 01-2 2H6a2 2 0 01-2-2V6a2 2 0 012-2h2M9 2h6a1 1 0 011 1v2a1 1 0 01-1 1H9a1 1 0 01-1-1V3a1 1 0 011-1z",
  chart: "M18 20V10M12 20V4M6 20v-6",
  wallet:
    "M3 7a2 2 0 012-2h14a2 2 0 012 2v12a2 2 0 01-2 2H5a2 2 0 01-2-2V7zM3 9h18M16 14h2",
  star: "M12 2l3.09 6.26L22 9.27l-5 4.87 1.18 6.88L12 17.77l-6.18 3.25L7 14.14 2 9.27l6.91-1.01L12 2z",
  store: "M3 9l1-5h16l1 5M3 9v11h18V9M3 9a3 3 0 006 0 3 3 0 006 0 3 3 0 006 0",
  chat: "M21 15a2 2 0 01-2 2H7l-4 4V5a2 2 0 012-2h14a2 2 0 012 2z",
  menu: "M3 6h18M3 12h18M3 18h18",
  "chevron-left": "M15 18l-6-6 6-6",
  "chevron-right": "M9 18l6-6-6-6",
};

/** Decorative stroke icon (the accessible name always comes from the label). */
export function NavIcon({
  name,
  className = "h-5 w-5",
}: { name: IconName; className?: string }): ReactNode {
  return (
    <svg
      aria-hidden="true"
      viewBox="0 0 24 24"
      className={`shrink-0 ${className}`}
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
    >
      <path d={PATHS[name]} />
    </svg>
  );
}

/** True when `pathname` belongs to the nav item (exact for the root, prefix otherwise). */
export function isNavActive(href: string, pathname: string): boolean {
  if (href === "/seller") {
    return pathname === "/seller" || /^\/seller\/[^/]+\/edit$/.test(pathname);
  }
  return pathname === href || pathname.startsWith(`${href}/`);
}
