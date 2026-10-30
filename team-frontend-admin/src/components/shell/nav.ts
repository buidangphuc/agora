export interface AdminNavItem {
  href: string;
  label: string;
  ready: boolean;
}

export const ADMIN_SCOPE = "admin";

export const ADMIN_HOME = "/admin/cockpit";

export const ADMIN_NAV: readonly AdminNavItem[] = [
  { href: "/admin/cockpit", label: "Tổng quan", ready: true },
  { href: "/admin/listings", label: "Kiểm duyệt tin đăng", ready: true },
  { href: "/admin/cases", label: "Hộp thư vụ việc", ready: true },
  { href: "/admin/users", label: "Người dùng", ready: true },
  { href: "/admin/audit", label: "Nhật ký kiểm toán", ready: true },
  { href: "/admin/vouchers", label: "Voucher sàn", ready: false },
  { href: "/admin/campaigns", label: "Chiến dịch", ready: true },
  { href: "/admin/plans", label: "Gói đăng ký", ready: true },
];

export function visibleNav(): AdminNavItem[] {
  return ADMIN_NAV.filter((item) => item.ready);
}
