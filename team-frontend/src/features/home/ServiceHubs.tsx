import Link from "next/link";

import { focusRing } from "@/components/ui/focus";

/** Shortcuts to routes that exist; every tile shares one neutral surface. */
const HUBS = [
  { name: "Kho voucher", icon: "🎟️", href: "/vouchers" },
  { name: "Freeship", icon: "🚚", href: "/vouchers?type=shipping" },
  { name: "Giảm tiền", icon: "💰", href: "/vouchers?type=fixed" },
  { name: "Giảm theo %", icon: "🏷️", href: "/vouchers?type=percent" },
  { name: "Tất cả sản phẩm", icon: "🛍️", href: "/search" },
  { name: "Đơn mua", icon: "📦", href: "/account/orders" },
  { name: "Yêu thích", icon: "❤️", href: "/favorites" },
  { name: "Shop theo dõi", icon: "🏪", href: "/account/following" },
];

export function ServiceHubs() {
  return (
    <nav
      aria-label="Lối tắt"
      className="rounded-3xl border border-border-subtle bg-surface-card p-3 shadow-card sm:p-5"
    >
      <ul className="grid grid-cols-4 gap-1 text-center sm:grid-cols-8 sm:gap-3">
        {HUBS.map((hub) => (
          <li key={hub.name} className="min-w-0">
            <Link
              href={hub.href}
              className={`group flex min-h-24 flex-col items-center justify-start gap-2 rounded-2xl px-1 py-2 transition duration-150 hover:bg-surface-muted ${focusRing}`}
            >
              <span
                aria-hidden="true"
                className="grid h-12 w-12 place-items-center rounded-2xl bg-primary-50 text-2xl"
              >
                {hub.icon}
              </span>
              <span className="line-clamp-2 text-xs font-medium leading-4 text-text-primary group-hover:text-action-primary sm:text-sm sm:leading-5">
                {hub.name}
              </span>
            </Link>
          </li>
        ))}
      </ul>
    </nav>
  );
}
