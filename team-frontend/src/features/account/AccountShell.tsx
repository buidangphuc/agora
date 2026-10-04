import Link from "next/link";
import type { ReactNode } from "react";

import { Breadcrumb } from "@/components/ui/Breadcrumb";
import { focusRing } from "@/components/ui/focus";

export type AccountMenuKey =
  | "addresses"
  | "security"
  | "verification"
  | "referral"
  | "following"
  | "favorites"
  | "notifications"
  | "orders";

export interface AccountMenuItem {
  key: AccountMenuKey;
  label: string;
  href: string;
}

/**
 * Menu entries shared by every account page. Exported so a page owned by
 * another phase (e.g. /account/orders) can adopt AccountShell without
 * re-declaring the list. "favorites" and "notifications" are standalone pages
 * that the menu only links to.
 */
export const ACCOUNT_MENU: AccountMenuItem[] = [
  { key: "addresses", label: "Địa chỉ", href: "/account/addresses" },
  { key: "security", label: "Bảo mật", href: "/account/security" },
  { key: "verification", label: "Xác minh", href: "/account/verification" },
  { key: "referral", label: "Giới thiệu", href: "/account/referral" },
  { key: "following", label: "Đang theo dõi", href: "/account/following" },
  { key: "favorites", label: "Yêu thích", href: "/favorites" },
  { key: "notifications", label: "Thông báo", href: "/notifications" },
  { key: "orders", label: "Đơn hàng", href: "/account/orders" },
];

export interface AccountShellProps {
  /** Which menu entry is the current page; becomes `aria-current="page"`. */
  current: AccountMenuKey;
  /** The page `h1`. */
  title: string;
  /** One line under the title. */
  description?: string;
  children: ReactNode;
}

const itemBase =
  "flex min-h-10 items-center whitespace-nowrap rounded-lg px-3 text-sm font-medium transition duration-150";
const itemIdle =
  "text-text-secondary hover:bg-surface-page hover:text-text-primary";
const itemActive = "bg-primary-50 text-action-primary";

/**
 * Account settings shell (Ant Design Pro: Account Settings). Server component:
 * the active menu entry comes from the `current` prop, never from client state.
 * It is deliberately not an `app/account/layout.tsx`, which would also wrap
 * /account/orders. The menu is a left column from `lg`, and one scrollable row
 * above the content on narrow screens.
 */
export function AccountShell({
  current,
  title,
  description,
  children,
}: AccountShellProps) {
  const currentLabel =
    ACCOUNT_MENU.find((item) => item.key === current)?.label ?? title;

  return (
    <div className="space-y-4 py-2">
      <Breadcrumb
        items={[
          { label: "Tài khoản", href: "/account/addresses" },
          { label: currentLabel },
        ]}
      />
      <header className="space-y-1">
        <h1 className="text-2xl font-bold text-text-primary">{title}</h1>
        {description && (
          <p className="text-sm text-text-secondary">{description}</p>
        )}
      </header>
      <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:gap-6">
        <nav aria-label="Menu tài khoản" className="lg:w-60 lg:shrink-0">
          <ul className="flex gap-1 overflow-x-auto lg:flex-col lg:overflow-visible">
            {ACCOUNT_MENU.map((item) => {
              const active = item.key === current;
              return (
                <li key={item.key} className="shrink-0">
                  <Link
                    href={item.href}
                    aria-current={active ? "page" : undefined}
                    className={`${itemBase} ${focusRing} ${active ? itemActive : itemIdle}`}
                  >
                    {item.label}
                  </Link>
                </li>
              );
            })}
          </ul>
        </nav>
        <div className="min-w-0 max-w-3xl flex-1 space-y-4">{children}</div>
      </div>
    </div>
  );
}
