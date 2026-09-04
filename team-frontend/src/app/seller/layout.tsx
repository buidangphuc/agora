import Link from "next/link";
import { redirect } from "next/navigation";
import type { ReactNode } from "react";

import { getPrincipal, hasScope } from "@/lib/gateway/session";

export default function SellerLayout({ children }: { children: ReactNode }) {
  const principal = getPrincipal();
  if (!principal) {
    redirect("/login");
  }

  if (!hasScope("listing.write")) {
    return (
      <div className="mx-auto max-w-lg py-16 text-center">
        <div className="rounded-xs border border-gray-200 bg-white p-8 shadow-xs">
          <span className="text-4xl">🛍️</span>
          <h1 className="mt-3 text-lg font-bold text-gray-900">
            Kích Hoạt Tài Khoản Kênh Người Bán
          </h1>
          <p className="mt-2 text-xs text-gray-500 leading-relaxed">
            Tài khoản của bạn hiện chưa đăng ký quyền Người Bán. Vui lòng kích
            hoạt để bắt đầu đăng bán sản phẩm.
          </p>
          <Link
            href="/"
            className="mt-6 inline-block rounded-xs bg-brand px-6 py-2 text-xs font-semibold text-white shadow-xs hover:bg-brand-dark"
          >
            Quay lại trang chủ Marketplace
          </Link>
        </div>
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-6 lg:flex-row -mx-4 -my-5 p-4 lg:p-6 min-h-[calc(100vh-140px)] bg-gray-50/50">
      {/* ── Seller Sidebar Navigation ── */}
      <aside className="w-full lg:w-60 shrink-0 space-y-4">
        {/* Shop Info Card */}
        <div className="rounded-xs border border-gray-200 bg-white p-4 shadow-2xs">
          <div className="flex items-center gap-3 border-b border-gray-100 pb-3">
            <div className="grid h-10 w-10 place-items-center rounded-full bg-orange-100 text-base font-bold text-brand">
              🏪
            </div>
            <div className="min-w-0 flex-1">
              <p className="truncate text-xs font-bold text-gray-900">
                {principal.name}
              </p>
              <span className="inline-block rounded-2xs bg-red-50 px-1.5 py-0.2 text-[9px] font-semibold text-brand">
                Official Store Partner
              </span>
            </div>
          </div>

          <div className="mt-3">
            <Link
              href="/"
              className="flex items-center justify-center gap-1.5 rounded-xs border border-gray-200 py-1.5 text-xs font-medium text-gray-600 hover:border-brand hover:text-brand transition"
            >
              <span>🌐</span>
              <span>Xem Gian Hàng Công Khai</span>
            </Link>
          </div>
        </div>

        {/* Navigation Menu */}
        <div className="rounded-xs border border-gray-200 bg-white p-3 shadow-2xs">
          <div className="px-3 py-1.5 text-[10px] font-bold uppercase tracking-wider text-gray-400">
            Quản Lý Sản Phẩm
          </div>
          <nav className="space-y-1 mt-1 text-xs">
            <Link
              href="/seller"
              className="flex items-center gap-2.5 rounded-xs px-3 py-2 font-medium text-gray-700 hover:bg-orange-50 hover:text-brand transition"
            >
              <span>📦</span>
              <span>Tất cả sản phẩm</span>
            </Link>

            <Link
              href="/seller/new"
              className="flex items-center gap-2.5 rounded-xs px-3 py-2 font-medium text-gray-700 hover:bg-orange-50 hover:text-brand transition"
            >
              <span>➕</span>
              <span>Thêm 1 sản phẩm mới</span>
            </Link>

            <Link
              href="/seller/bundles"
              className="flex items-center gap-2.5 rounded-xs px-3 py-2 font-medium text-gray-700 hover:bg-orange-50 hover:text-brand transition"
            >
              <span>🧩</span>
              <span>Combo sản phẩm</span>
            </Link>

            <Link
              href="/seller/ads"
              className="flex items-center gap-2.5 rounded-xs px-3 py-2 font-medium text-gray-700 hover:bg-orange-50 hover:text-brand transition"
            >
              <span>📣</span>
              <span>Quảng cáo</span>
            </Link>

            <Link
              href="/seller/orders"
              className="flex items-center gap-2.5 rounded-xs px-3 py-2 font-medium text-gray-700 hover:bg-orange-50 hover:text-brand transition"
            >
              <span>📑</span>
              <span>Quản lý đơn hàng</span>
            </Link>

            <Link
              href="/seller/analytics"
              className="flex items-center gap-2.5 rounded-xs px-3 py-2 font-medium text-gray-700 hover:bg-orange-50 hover:text-brand transition"
            >
              <span>📊</span>
              <span>Báo cáo doanh thu</span>
            </Link>

            <Link
              href="/seller/wallet"
              className="flex items-center gap-2.5 rounded-xs px-3 py-2 font-medium text-gray-700 hover:bg-orange-50 hover:text-brand transition"
            >
              <span>💰</span>
              <span>Ví người bán</span>
            </Link>

            <Link
              href="/seller/plans"
              className="flex items-center gap-2.5 rounded-xs px-3 py-2 font-medium text-gray-700 hover:bg-orange-50 hover:text-brand transition"
            >
              <span>⭐</span>
              <span>Gói đăng ký</span>
            </Link>

            <Link
              href="/chat"
              className="flex items-center gap-2.5 rounded-xs px-3 py-2 font-medium text-gray-700 hover:bg-orange-50 hover:text-brand transition"
            >
              <span>💬</span>
              <span>Chăm sóc khách hàng</span>
            </Link>
          </nav>
        </div>
      </aside>

      {/* ── Main Content Area ── */}
      <div className="flex-1 min-w-0">{children}</div>
    </div>
  );
}
