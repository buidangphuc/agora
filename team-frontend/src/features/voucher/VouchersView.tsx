"use client";

import Link from "next/link";
import { useState } from "react";

import { useToast } from "@/components/ui/ToastProvider";
import { AVAILABLE_VOUCHERS, type Voucher } from "@/lib/vouchers";

export function VouchersView() {
  const { showToast } = useToast();
  const [savedCodes, setSavedCodes] = useState<string[]>(["FREESHIP"]);
  const [categoryTab, setCategoryTab] = useState<string>("all");

  function handleSaveVoucher(voucher: Voucher) {
    if (savedCodes.includes(voucher.code)) {
      showToast("Voucher này đã có trong ví của bạn!", "info");
      return;
    }
    setSavedCodes((prev) => [...prev, voucher.code]);
    showToast(`✓ Đã lưu mã [${voucher.code}] vào ví voucher!`, "success");
  }

  const filteredVouchers =
    categoryTab === "all"
      ? AVAILABLE_VOUCHERS
      : categoryTab === "shipping"
        ? AVAILABLE_VOUCHERS.filter((v) => v.discountType === "shipping")
        : categoryTab === "fixed"
          ? AVAILABLE_VOUCHERS.filter((v) => v.discountType === "fixed")
          : AVAILABLE_VOUCHERS.filter((v) => v.discountType === "percent");

  return (
    <div className="space-y-6">
      {/* ── Banner ── */}
      <div className="relative overflow-hidden rounded-2xl bg-gradient-to-r from-orange-600 via-brand to-red-500 p-6 md:p-8 text-white shadow-md">
        <div className="max-w-xl">
          <span className="rounded-full bg-white/20 px-3 py-1 text-xs font-bold uppercase tracking-wider backdrop-blur-xs">
            🎟️ KHO VOUCHER SIÊU SALE
          </span>
          <h1 className="mt-3 text-2xl md:text-3xl font-black leading-tight">
            SĂN MÃ GIẢM GIÁ <br />
            <span className="text-yellow-300">FREESHIP 0Đ & HOÀN XU 20%</span>
          </h1>
          <p className="mt-2 text-xs md:text-sm text-orange-100">
            Lưu voucher vào ví ngay hôm nay để tự động giảm giá khi thanh toán
            đơn hàng.
          </p>
        </div>
      </div>

      {/* ── Category Tabs ── */}
      <div className="flex border-b border-gray-200 bg-white p-2 rounded-xl shadow-2xs text-xs font-semibold text-gray-600 gap-2">
        <button
          type="button"
          onClick={() => setCategoryTab("all")}
          className={`py-2 px-4 rounded-lg transition ${
            categoryTab === "all"
              ? "bg-brand text-white font-bold shadow-2xs"
              : "hover:bg-gray-100"
          }`}
        >
          Tất cả voucher ({AVAILABLE_VOUCHERS.length})
        </button>
        <button
          type="button"
          onClick={() => setCategoryTab("shipping")}
          className={`py-2 px-4 rounded-lg transition ${
            categoryTab === "shipping"
              ? "bg-brand text-white font-bold shadow-2xs"
              : "hover:bg-gray-100"
          }`}
        >
          ⚡ Miễn phí vận chuyển
        </button>
        <button
          type="button"
          onClick={() => setCategoryTab("fixed")}
          className={`py-2 px-4 rounded-lg transition ${
            categoryTab === "fixed"
              ? "bg-brand text-white font-bold shadow-2xs"
              : "hover:bg-gray-100"
          }`}
        >
          💰 Giảm giá tiền mặt
        </button>
        <button
          type="button"
          onClick={() => setCategoryTab("percent")}
          className={`py-2 px-4 rounded-lg transition ${
            categoryTab === "percent"
              ? "bg-brand text-white font-bold shadow-2xs"
              : "hover:bg-gray-100"
          }`}
        >
          🏷️ Giảm % Đơn hàng
        </button>
      </div>

      {/* ── Voucher Grid ── */}
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-2">
        {filteredVouchers.map((v, i) => {
          const isSaved = savedCodes.includes(v.code);
          const usedPercent = 45 + ((i * 15) % 45);

          return (
            <div
              key={v.code}
              className="relative flex overflow-hidden rounded-xl border border-orange-200 bg-white shadow-xs transition hover:shadow-md"
            >
              {/* Left Badge Strip */}
              <div className="flex w-28 shrink-0 flex-col items-center justify-center border-r border-dashed border-orange-200 bg-orange-50/80 p-3 text-center">
                <span className="text-2xl">
                  {v.discountType === "shipping" ? "🚚" : "🎟️"}
                </span>
                <span className="mt-1 text-[11px] font-bold text-brand uppercase">
                  {v.badge}
                </span>
              </div>

              {/* Right Details */}
              <div className="flex flex-1 flex-col justify-between p-4 min-w-0">
                <div>
                  <div className="flex items-center justify-between gap-2">
                    <span className="rounded bg-brand/10 px-2 py-0.5 text-xs font-bold text-brand">
                      {v.code}
                    </span>
                    <span className="text-[10px] text-gray-400">
                      HSD: 30 ngày
                    </span>
                  </div>
                  <h3 className="mt-1.5 text-sm font-bold text-gray-900 truncate">
                    {v.title}
                  </h3>
                  <p className="text-xs text-gray-500 line-clamp-1 mt-0.5">
                    {v.description}
                  </p>
                </div>

                {/* Progress bar and CTA */}
                <div className="mt-3 flex items-center justify-between gap-4 pt-2 border-t border-gray-100">
                  <div className="flex-1">
                    <div className="flex justify-between text-[10px] text-gray-400 mb-1">
                      <span>Đã dùng {usedPercent}%</span>
                    </div>
                    <div className="h-1.5 w-full overflow-hidden rounded-full bg-gray-100">
                      <div
                        className="h-full bg-brand rounded-full"
                        style={{ width: `${usedPercent}%` }}
                      />
                    </div>
                  </div>

                  <div className="shrink-0 flex items-center gap-2">
                    <button
                      type="button"
                      onClick={() => handleSaveVoucher(v)}
                      className={`rounded-lg px-3.5 py-1.5 text-xs font-semibold transition ${
                        isSaved
                          ? "bg-gray-100 text-gray-600 cursor-default"
                          : "bg-orange-50 text-brand border border-brand/30 hover:bg-brand hover:text-white"
                      }`}
                    >
                      {isSaved ? "✓ Đã lưu" : "Lưu mã"}
                    </button>
                    {isSaved && (
                      <Link
                        href="/search"
                        className="rounded-lg bg-brand px-3 py-1.5 text-xs font-semibold text-white hover:bg-brand-dark transition"
                      >
                        Dùng ngay
                      </Link>
                    )}
                  </div>
                </div>
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}
