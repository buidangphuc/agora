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
      className="rounded-2xl border border-border-subtle bg-surface-card p-4 shadow-preline-card"
    >
      <ul className="grid grid-cols-4 gap-2 text-center sm:grid-cols-8">
        {HUBS.map((hub) => (
          <li key={hub.name}>
            <Link
              href={hub.href}
              className={`group flex min-h-20 flex-col items-center justify-start gap-1.5 rounded-xl p-2 transition duration-150 hover:bg-surface-muted ${focusRing}`}
            >
              <span
                aria-hidden="true"
                className="grid h-12 w-12 place-items-center rounded-2xl border border-border-subtle bg-surface-muted text-2xl"
              >
                {hub.icon}
              </span>
              <span className="line-clamp-2 text-xs font-medium leading-4 text-text-primary group-hover:text-action-primary">
                {hub.name}
              </span>
            </Link>
          </li>
        ))}
      </ul>
    </nav>
  );
}
