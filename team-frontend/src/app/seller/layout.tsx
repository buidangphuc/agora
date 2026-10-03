import Link from "next/link";
import { redirect } from "next/navigation";
import type { ReactNode } from "react";

import { Badge } from "@/components/ui/Badge";
import { Button } from "@/components/ui/Button";
import { Card } from "@/components/ui/Card";
import { getPrincipal, hasScope } from "@/lib/gateway/session";

export default function SellerLayout({ children }: { children: ReactNode }) {
  const principal = getPrincipal();
  if (!principal) {
    redirect("/login");
  }

  if (!hasScope("listing.write")) {
    return (
      <div className="mx-auto max-w-lg py-16 text-center">
        <Card className="rounded-2xl p-8 border-gray-200/90 shadow-preline-card">
          <span className="text-4xl">🛍️</span>
          <h1 className="mt-3 text-lg font-bold text-gray-900">
            Kích Hoạt Tài Khoản Kênh Người Bán
          </h1>
          <p className="mt-2 text-xs text-gray-500 leading-relaxed">
            Tài khoản của bạn hiện chưa đăng ký quyền Người Bán. Vui lòng kích
            hoạt để bắt đầu đăng bán sản phẩm.
          </p>
          <div className="mt-6">
            <Link href="/">
              <Button variant="primary" size="md">
                Quay lại trang chủ Marketplace
              </Button>
            </Link>
          </div>
        </Card>
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-6 lg:flex-row -mx-4 -my-5 p-4 lg:p-6 min-h-[calc(100vh-140px)] bg-gray-50/60">
      {/* ── Seller Sidebar Navigation ── */}
      <aside className="w-full lg:w-64 shrink-0 space-y-4">
        {/* Shop Info Card */}
        <Card className="rounded-2xl p-4 border-gray-200/80 shadow-preline-card">
          <div className="flex items-center gap-3 border-b border-gray-100 pb-3">
            <div className="grid h-11 w-11 place-items-center rounded-xl bg-primary-50 text-xl font-bold text-primary-600 shadow-2xs">
              🏪
            </div>
            <div className="min-w-0 flex-1">
              <p className="truncate text-xs font-bold text-gray-900">
                {principal.name}
              </p>
              <div className="mt-1">
                <Badge variant="primary" size="xs">
                  Official Store Partner
                </Badge>
              </div>
            </div>
          </div>

          <div className="mt-3">
            <Link href="/" className="block">
              <Button
                variant="outline"
                size="sm"
                className="w-full text-xs font-medium text-gray-700"
              >
                🌐 Xem Gian Hàng Công Khai
              </Button>
            </Link>
          </div>
        </Card>

        {/* Navigation Menu */}
        <Card className="rounded-2xl p-3 border-gray-200/80 shadow-preline-card">
          <div className="px-3 py-1.5 text-[10px] font-bold uppercase tracking-wider text-gray-400">
            Quản Lý Gian Hàng
          </div>
          <nav className="space-y-1 mt-1 text-xs">
            <Link
              href="/seller"
              className="flex items-center gap-2.5 rounded-lg px-3 py-2 font-medium text-gray-700 hover:bg-primary-50 hover:text-primary-600 transition"
            >
              <span>📦</span>
              <span>Tất cả sản phẩm</span>
            </Link>

            <Link
              href="/seller/new"
              className="flex items-center gap-2.5 rounded-lg px-3 py-2 font-medium text-gray-700 hover:bg-primary-50 hover:text-primary-600 transition"
            >
              <span>➕</span>
              <span>Thêm sản phẩm mới</span>
            </Link>

            <Link
              href="/seller/bundles"
              className="flex items-center gap-2.5 rounded-lg px-3 py-2 font-medium text-gray-700 hover:bg-primary-50 hover:text-primary-600 transition"
            >
              <span>🧩</span>
              <span>Combo sản phẩm</span>
            </Link>

            <Link
              href="/seller/ads"
              className="flex items-center gap-2.5 rounded-lg px-3 py-2 font-medium text-gray-700 hover:bg-primary-50 hover:text-primary-600 transition"
            >
              <span>📣</span>
              <span>Quảng cáo</span>
            </Link>

            <Link
              href="/seller/orders"
              className="flex items-center gap-2.5 rounded-lg px-3 py-2 font-medium text-gray-700 hover:bg-primary-50 hover:text-primary-600 transition"
            >
              <span>📑</span>
              <span>Quản lý đơn hàng</span>
            </Link>

            <Link
              href="/seller/analytics"
              className="flex items-center gap-2.5 rounded-lg px-3 py-2 font-medium text-gray-700 hover:bg-primary-50 hover:text-primary-600 transition"
            >
              <span>📊</span>
              <span>Báo cáo doanh thu</span>
            </Link>

            <Link
              href="/seller/wallet"
              className="flex items-center gap-2.5 rounded-lg px-3 py-2 font-medium text-gray-700 hover:bg-primary-50 hover:text-primary-600 transition"
            >
              <span>💰</span>
              <span>Ví người bán</span>
            </Link>

            <Link
              href="/seller/plans"
              className="flex items-center gap-2.5 rounded-lg px-3 py-2 font-medium text-gray-700 hover:bg-primary-50 hover:text-primary-600 transition"
            >
              <span>⭐</span>
              <span>Gói đăng ký</span>
            </Link>

            <Link
              href="/chat"
              className="flex items-center gap-2.5 rounded-lg px-3 py-2 font-medium text-gray-700 hover:bg-primary-50 hover:text-primary-600 transition"
            >
              <span>💬</span>
              <span>Chăm sóc khách hàng</span>
            </Link>
          </nav>
        </Card>
      </aside>

      {/* ── Main Content Area ── */}
      <div className="flex-1 min-w-0">{children}</div>
    </div>
  );
}
