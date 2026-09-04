import "./globals.css";

import Link from "next/link";
import type { ReactNode } from "react";

import { FloatingChatBubble } from "@/components/ui/FloatingChatBubble";
import { SearchBar } from "@/features/search/SearchBar";
import { ToastProvider } from "@/components/ui/ToastProvider";
import { logoutAction } from "@/features/auth/actions";
import { getCart } from "@/lib/gateway/cart";
import { getPrincipal, hasScope } from "@/lib/gateway/session";

export const metadata = {
  title: "Marketplace Showcase | Nền Tảng Thương Mại Điện Tử Polyrepo",
  description:
    "Dự án thương mại điện tử kiến trúc Polyrepo, Microservices gRPC, Event-driven Kafka, OpenSearch và Next.js SSR.",
};

async function AuthNav() {
  const principal = getPrincipal();
  if (!principal) {
    return (
      <div className="flex items-center gap-3 text-[13px] text-white whitespace-nowrap">
        <Link
          href="/register"
          className="hover:text-white/80 font-medium transition"
        >
          Đăng Ký
        </Link>
        <span className="text-white/40">|</span>
        <Link
          href="/login"
          className="hover:text-white/80 font-medium transition"
        >
          Đăng Nhập
        </Link>
      </div>
    );
  }

  return (
    <div className="flex items-center gap-3 text-[13px] text-white whitespace-nowrap">
      <Link href="/account/orders" className="hover:text-white/80 transition">
        Đơn Mua
      </Link>
      <span className="text-white/30">|</span>
      <Link
        href="/account/following"
        className="hover:text-white/80 transition"
      >
        Đang Theo Dõi
      </Link>
      <span className="text-white/30">|</span>
      <Link href="/account/referral" className="hover:text-white/80 transition">
        Mời bạn
      </Link>
      <span className="text-white/30">|</span>
      <Link
        href="/account/verification"
        className="hover:text-white/80 transition"
      >
        Xác minh
      </Link>
      <span className="text-white/30">|</span>
      <Link href="/account/security" className="hover:text-white/80 transition">
        Bảo Mật
      </Link>
      <span className="text-white/30">|</span>
      <div className="flex items-center gap-1.5 font-semibold">
        <div className="grid h-5 w-5 place-items-center rounded-full bg-white/20 text-[10px] text-white">
          👤
        </div>
        <span className="max-w-[120px] truncate">{principal.name}</span>
      </div>
      <form action={logoutAction} className="inline">
        <button
          type="submit"
          className="text-white/70 hover:text-white underline text-xs cursor-pointer ml-1"
        >
          Đăng xuất
        </button>
      </form>
    </div>
  );
}

export default async function RootLayout({
  children,
}: { children: ReactNode }) {
  const cart = await getCart().catch(() => ({ totalItems: 0 }));

  return (
    <html lang="vi">
      <body className="bg-[#f5f5f5] text-[#222222] flex flex-col min-h-screen font-sans antialiased">
        <ToastProvider>
          {/* ── Shopee Signature Gradient Header ── */}
          <header className="bg-gradient-to-b from-[#f53d2d] to-[#f63] text-white sticky top-0 z-30 shadow-md">
            {/* 1. Top Utility Navigation Bar */}
            <div className="text-[13px] py-1.5 border-b border-white/10">
              <div className="mx-auto flex max-w-[1200px] items-center justify-between px-4">
                {/* Left Side Links */}
                <div className="flex items-center gap-3 text-white/90 whitespace-nowrap">
                  <Link
                    href="/seller"
                    className="hover:text-white transition font-normal"
                  >
                    Kênh Người Bán
                  </Link>
                  <span className="text-white/30">|</span>
                  <Link
                    href="/assistant"
                    className="hover:text-white transition font-bold bg-white/20 px-2 py-0.5 rounded-full flex items-center gap-1 shadow-2xs"
                  >
                    <span>🤖</span>
                    <span>AI Assistant</span>
                  </Link>
                  <span className="text-white/30">|</span>
                  <div className="flex items-center gap-1.5">
                    <span>Kết nối</span>
                    <a
                      href="https://facebook.com"
                      target="_blank"
                      rel="noreferrer"
                      className="hover:opacity-80"
                      aria-label="Facebook"
                    >
                      <svg
                        className="w-3.5 h-3.5 fill-white"
                        viewBox="0 0 24 24"
                        role="img"
                        aria-label="Facebook"
                      >
                        <title>Facebook</title>
                        <path d="M22 12c0-5.52-4.48-10-10-10S2 6.48 2 12c0 4.84 3.44 8.87 8 9.8V15H8v-3h2V9.5C10 7.57 11.57 6 13.5 6H16v3h-2c-.55 0-1 .45-1 1v2h3v3h-3v6.95C18.05 21.45 22 17.19 22 12z" />
                      </svg>
                    </a>
                    <a
                      href="https://instagram.com"
                      target="_blank"
                      rel="noreferrer"
                      className="hover:opacity-80"
                      aria-label="Instagram"
                    >
                      <svg
                        className="w-3.5 h-3.5 fill-white"
                        viewBox="0 0 24 24"
                        role="img"
                        aria-label="Instagram"
                      >
                        <title>Instagram</title>
                        <path d="M12 2.163c3.204 0 3.584.012 4.85.07 3.252.148 4.771 1.691 4.919 4.919.058 1.265.069 1.645.069 4.849 0 3.205-.012 3.584-.069 4.849-.149 3.225-1.664 4.771-4.919 4.919-1.266.058-1.644.07-4.85.07-3.204 0-3.584-.012-4.849-.07-3.26-.149-4.771-1.699-4.919-4.92-.058-1.265-.07-1.644-.07-4.849 0-3.204.013-3.583.07-4.849.149-3.227 1.664-4.771 4.919-4.919 1.266-.057 1.645-.069 4.849-.069zm0-2.163c-3.259 0-3.667.014-4.947.072-4.358.2-6.78 2.618-6.98 6.98-.059 1.281-.073 1.689-.073 4.948 0 3.259.014 3.668.072 4.948.2 4.358 2.618 6.78 6.98 6.98 1.281.058 1.689.072 4.948.072 3.259 0 3.668-.014 4.948-.072 4.354-.2 6.782-2.618 6.979-6.98.059-1.28.073-1.689.073-4.948 0-3.259-.014-3.667-.072-4.947-.196-4.354-2.617-6.78-6.979-6.98-1.281-.059-1.69-.073-4.949-.073zm0 5.838c-3.403 0-6.162 2.759-6.162 6.162s2.759 6.163 6.162 6.163 6.162-2.759 6.162-6.163c0-3.403-2.759-6.162-6.162-6.162zm0 10.162c-2.209 0-4-1.79-4-4 0-2.209 1.791-4 4-4s4 1.791 4 4c0 2.21-1.791 4-4 4zm6.406-11.845c-.796 0-1.441.645-1.441 1.44s.645 1.44 1.441 1.44c.795 0 1.439-.645 1.439-1.44s-.644-1.44-1.439-1.44z" />
                      </svg>
                    </a>
                  </div>
                </div>

                {/* Right Side Links */}
                <div className="flex items-center gap-4 text-white/90 whitespace-nowrap">
                  <Link
                    href="/notifications"
                    className="flex items-center gap-1 hover:text-white transition"
                  >
                    <svg
                      className="w-3.5 h-3.5 fill-current"
                      viewBox="0 0 16 16"
                      role="img"
                      aria-label="Thông báo"
                    >
                      <title>Thông báo</title>
                      <path d="M8 16a2 2 0 0 0 2-2H6a2 2 0 0 0 2 2zM14.2 11.2c-.6-.7-1.7-1.5-1.7-4.5 0-2.6-1.8-4.7-4.2-5.1V1a.8.8 0 0 0-1.6 0v.6C4.3 2 2.5 4.1 2.5 6.7c0 3-1.1 3.8-1.7 4.5-.3.4-.4.8-.4 1.1 0 .9.7 1.7 1.6 1.7h12c.9 0 1.6-.8 1.6-1.7 0-.3-.1-.7-.4-1.1z" />
                    </svg>
                    <span>Thông Báo</span>
                  </Link>
                  <Link
                    href="/vouchers"
                    className="hover:text-white transition flex items-center gap-1"
                  >
                    <svg
                      className="w-3.5 h-3.5 fill-current"
                      viewBox="0 0 24 24"
                      role="img"
                      aria-label="Kho Voucher"
                    >
                      <title>Kho Voucher</title>
                      <path d="M20 4H4c-1.11 0-1.99.89-1.99 2L2 18c0 1.11.89 2 2 2h16c1.11 0 2-.89 2-2V6c0-1.11-.89-2-2-2zm0 14H4v-3.5c.83 0 1.5-.67 1.5-1.5s-.67-1.5-1.5-1.5V6h16v3.5c-.83 0-1.5.67-1.5 1.5s.67 1.5 1.5 1.5V18z" />
                    </svg>
                    <span>Kho Voucher</span>
                  </Link>
                  <span className="hover:text-white transition cursor-pointer flex items-center gap-1">
                    <svg
                      className="w-3.5 h-3.5 fill-current"
                      viewBox="0 0 24 24"
                      role="img"
                      aria-label="Hỗ Trợ"
                    >
                      <title>Hỗ Trợ</title>
                      <path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm1 16h-2v-2h2v2zm1.07-7.75l-.9.92C12.45 11.9 12 12.5 12 14h-2v-.5c0-1.1.45-2.1 1.17-2.83l1.24-1.26c.37-.36.59-.86.59-1.41 0-1.1-.9-2-2-2s-2 .9-2 2H7c0-2.76 2.24-5 5-5s5 2.24 5 5c0 1.04-.42 1.99-1.07 2.75z" />
                    </svg>
                    <span>Hỗ Trợ</span>
                  </span>
                  <span className="hover:text-white transition cursor-pointer flex items-center gap-1">
                    <svg
                      className="w-3.5 h-3.5 fill-current"
                      viewBox="0 0 24 24"
                      role="img"
                      aria-label="Tiếng Việt"
                    >
                      <title>Tiếng Việt</title>
                      <path d="M11.99 2C6.47 2 2 6.48 2 12s4.47 10 9.99 10C17.52 22 22 17.52 22 12S17.52 2 11.99 2zm6.93 6h-2.95c-.32-1.25-.78-2.45-1.38-3.56 1.84.63 3.37 1.91 4.33 3.56zM12 4.04c.83 1.2 1.48 2.53 1.91 3.96h-3.82c.43-1.43 1.08-2.76 1.91-3.96zM4.26 14C4.1 13.36 4 12.69 4 12s.1-1.36.26-2h3.38c-.08.66-.14 1.32-.14 2 0 .68.06 1.34.14 2H4.26zm.82 2h2.95c.32 1.25.78 2.45 1.38 3.56-1.84-.63-3.37-1.9-4.33-3.56zm2.95-8H5.08c.96-1.66 2.49-2.93 4.33-3.56C8.81 5.55 8.35 6.75 8.03 8zM12 19.96c-.83-1.2-1.48-2.53-1.91-3.96h3.82c-.43 1.43-1.08 2.76-1.91 3.96zM14.34 14H9.66c-.09-.66-.16-1.32-.16-2 0-.68.07-1.35.16-2h4.68c.09.65.16 1.32.16 2 0 .68-.07 1.34-.16 2zm.25 5.56c.6-1.11 1.06-2.31 1.38-3.56h2.95c-.96 1.65-2.49 2.93-4.33 3.56zM16.36 14c.08-.66.14-1.32.14-2 0-.68-.06-1.34-.14-2h3.38c.16.64.26 1.31.26 2s-.1 1.36-.26 2h-3.38z" />
                    </svg>
                    <span>Tiếng Việt</span>
                  </span>
                  <span className="text-white/30">|</span>
                  <AuthNav />
                </div>
              </div>
            </div>

            {/* 2. Main Search & Brand Bar */}
            <div className="mx-auto flex max-w-[1200px] items-center justify-between gap-8 px-4 pt-3.5 pb-2.5">
              {/* Logo */}
              <Link
                href="/"
                className="flex items-center gap-2.5 text-white shrink-0 hover:opacity-95 transition"
              >
                <div className="relative grid h-10 w-10 place-items-center rounded-xl bg-white/20 text-xl font-bold text-white shadow-inner">
                  🛍️
                </div>
                <div className="flex flex-col">
                  <span className="font-black text-2xl tracking-tight leading-none">
                    Marketplace
                  </span>
                  <span className="text-[10px] font-bold tracking-wider text-yellow-200 uppercase mt-0.5">
                    AI Polyrepo Platform
                  </span>
                </div>
              </Link>

              {/* Search Bar with pills */}
              <div className="flex-1 max-w-3xl">
                <div className="w-full">
                  <SearchBar />
                </div>
              </div>

              {/* Cart Icon with Live Counter */}
              <Link
                href="/cart"
                className="relative flex items-center p-2 text-white hover:opacity-90 shrink-0 transition"
              >
                <svg
                  className="w-7 h-7 fill-white"
                  viewBox="0 0 24 24"
                  role="img"
                  aria-label="Giỏ hàng"
                >
                  <title>Giỏ hàng</title>
                  <path d="M10 19.5c0 .829-.672 1.5-1.5 1.5s-1.5-.671-1.5-1.5c0-.828.672-1.5 1.5-1.5s1.5.672 1.5 1.5zm3.5-1.5c-.828 0-1.5.671-1.5 1.5s.672 1.5 1.5 1.5 1.5-.671 1.5-1.5c0-.828-.672-1.5-1.5-1.5zm1.33-2l.9-5h-10.33l-.4-2h-2v2h1.24l1.6 8h10.45l-.46-3zm-1.83-6l-.54 3h-8.06l-.6-3h9.2z" />
                </svg>
                {cart.totalItems > 0 && (
                  <span className="absolute -top-1 right-0 grid h-5 min-w-5 place-items-center rounded-full bg-white px-1 text-[11px] font-black text-brand shadow-md">
                    {cart.totalItems}
                  </span>
                )}
              </Link>
            </div>
          </header>

          {/* ── Architectural & Educational Disclaimer Banner ── */}
          <div className="bg-amber-50 border-b border-amber-200 text-slate-800 text-xs py-2.5 px-4 shadow-xs">
            <div className="mx-auto max-w-[1200px] flex flex-col sm:flex-row sm:items-center justify-between gap-2">
              <div className="flex items-start sm:items-center gap-2.5">
                <span className="text-base leading-none">💡</span>
                <span className="text-[12px] text-slate-700 leading-relaxed">
                  <strong className="text-amber-950 font-bold">
                    Tuyên bố đồ án & nghiên cứu kiến trúc:
                  </strong>{" "}
                  Toàn bộ giao diện và các luồng trải nghiệm này được xây dựng
                  hoàn toàn vì mục đích{" "}
                  <em>
                    học tập, nghiên cứu kiến trúc Polyrepo, Microservices gRPC,
                    Event-Driven Kafka, CQRS & Saga Pattern
                  </em>{" "}
                  (tham khảo từ các mô hình TMĐT phổ biến),{" "}
                  <strong>
                    phi thương mại và hoàn toàn không có ý định sao chép/clone
                    thương hiệu
                  </strong>
                  .
                </span>
              </div>
              <span className="inline-block self-start sm:self-auto text-[10px] bg-amber-200/90 text-amber-950 font-bold px-2 py-0.5 rounded uppercase tracking-wider shrink-0">
                Polyrepo Showcase
              </span>
            </div>
          </div>

          {/* ── Main Content Area ── */}
          <main className="mx-auto max-w-[1200px] px-4 py-5 flex-1 w-full">
            {children}
          </main>

          {/* Floating Chat Bubble */}
          <FloatingChatBubble />

          {/* ── Footer ── */}
          <footer className="mt-16 border-t-4 border-brand bg-white text-gray-600 text-xs">
            <div className="mx-auto max-w-[1200px] px-4 py-10 grid grid-cols-2 md:grid-cols-5 gap-8">
              <div>
                <h4 className="font-bold text-gray-800 uppercase mb-3 text-xs tracking-wider">
                  CHĂM SÓC KHÁCH HÀNG
                </h4>
                <ul className="space-y-2 text-gray-500 text-xs">
                  <li>
                    <Link href="/search" className="hover:text-brand">
                      Trung Tâm Trợ Giúp
                    </Link>
                  </li>
                  <li>
                    <Link href="/search" className="hover:text-brand">
                      Hướng Dẫn Mua Hàng
                    </Link>
                  </li>
                  <li>
                    <Link href="/account/orders" className="hover:text-brand">
                      Tra Cứu Đơn Hàng & Vận Đơn
                    </Link>
                  </li>
                  <li>
                    <Link href="/account/orders" className="hover:text-brand">
                      Chính Sách Trả Hàng & Hoàn Tiền (RMA)
                    </Link>
                  </li>
                  <li>
                    <Link href="/chat" className="hover:text-brand">
                      Chăm Sóc Khách Hàng 24/7
                    </Link>
                  </li>
                </ul>
              </div>

              <div>
                <h4 className="font-bold text-gray-800 uppercase mb-3 text-xs tracking-wider">
                  VỀ MARKETPLACE
                </h4>
                <ul className="space-y-2 text-gray-500 text-xs">
                  <li>
                    <Link href="/" className="hover:text-brand">
                      Giới Thiệu Đồ Án
                    </Link>
                  </li>
                  <li>
                    <Link href="/admin/cockpit" className="hover:text-brand">
                      Admin Observability Cockpit
                    </Link>
                  </li>
                  <li>
                    <Link href="/seller" className="hover:text-brand">
                      Kênh Người Bán & Quản Lý Kho
                    </Link>
                  </li>
                  <li>
                    <Link href="/vouchers" className="hover:text-brand">
                      Kho Voucher Khuyến Mãi
                    </Link>
                  </li>
                </ul>
              </div>

              <div>
                <h4 className="font-bold text-gray-800 uppercase mb-3 text-xs tracking-wider">
                  THANH TOÁN
                </h4>
                <div className="grid grid-cols-3 gap-2 text-gray-600 text-[11px] font-medium">
                  <span className="p-1.5 border border-gray-200 rounded bg-white text-center shadow-2xs font-bold">
                    VietQR
                  </span>
                  <span className="p-1.5 border border-gray-200 rounded bg-white text-center shadow-2xs">
                    Visa
                  </span>
                  <span className="p-1.5 border border-gray-200 rounded bg-white text-center shadow-2xs">
                    Master
                  </span>
                  <span className="p-1.5 border border-gray-200 rounded bg-white text-center shadow-2xs">
                    MoMo
                  </span>
                  <span className="p-1.5 border border-gray-200 rounded bg-white text-center shadow-2xs">
                    COD
                  </span>
                  <span className="p-1.5 border border-gray-200 rounded bg-white text-center shadow-2xs">
                    ZaloPay
                  </span>
                </div>
              </div>

              <div>
                <h4 className="font-bold text-gray-800 uppercase mb-3 text-xs tracking-wider">
                  ĐƠN VỊ VẬN CHUYỂN
                </h4>
                <div className="grid grid-cols-2 gap-2 text-gray-600 text-[11px] font-medium">
                  <span className="p-1.5 border border-gray-200 rounded bg-white text-center font-bold text-brand shadow-2xs">
                    SPX Express
                  </span>
                  <span className="p-1.5 border border-gray-200 rounded bg-white text-center shadow-2xs">
                    GHTK
                  </span>
                  <span className="p-1.5 border border-gray-200 rounded bg-white text-center shadow-2xs">
                    GHN
                  </span>
                  <span className="p-1.5 border border-gray-200 rounded bg-white text-center shadow-2xs">
                    Viettel Post
                  </span>
                </div>
              </div>

              <div>
                <h4 className="font-bold text-gray-800 uppercase mb-3 text-xs tracking-wider">
                  KIẾN TRÚC HỆ THỐNG
                </h4>
                <div className="space-y-1.5 text-gray-500 text-[11px]">
                  <p className="font-bold text-slate-800">
                    Polyrepo Microservices
                  </p>
                  <p>• Next.js 14 App Router SSR</p>
                  <p>• Envoy / Connect Edge Gateway</p>
                  <p>• 8 Go gRPC Services</p>
                  <p>• Kafka CQRS & OpenSearch</p>
                </div>
              </div>
            </div>

            <div className="border-t bg-gray-50 py-6 text-center text-gray-500 text-[11px] space-y-1">
              <p>
                © 2026 Marketplace Showcase Polyrepo. Xây dựng phục vụ mục đích
                học tập & nghiên cứu kiến trúc hệ thống.
              </p>
            </div>
          </footer>
        </ToastProvider>
      </body>
    </html>
  );
}
