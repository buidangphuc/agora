"use client";

import Link from "next/link";
import { useState } from "react";

export function SellerAnalyticsMock() {
  const [payoutLoading, setPayoutLoading] = useState(false);
  const [payoutSuccess, setPayoutSuccess] = useState(false);

  const handlePayout = () => {
    setPayoutLoading(true);
    setTimeout(() => {
      setPayoutLoading(false);
      setPayoutSuccess(true);
      setTimeout(() => setPayoutSuccess(false), 4000);
    }, 1200);
  };

  return (
    <div className="space-y-6">
      {/* Top Header */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4 bg-white p-6 rounded-2xl border border-slate-100 shadow-sm">
        <div>
          <h1 className="text-2xl font-black text-slate-900 tracking-tight flex items-center gap-2">
            📊 Kênh Phân Tích & Ví Doanh Thu
          </h1>
          <p className="text-sm text-slate-500 mt-1">
            Tổng quan hiệu quả kinh doanh, tỷ lệ chuyển đổi và số dư ví người
            bán
          </p>
        </div>
        <div className="flex items-center gap-3">
          <button
            type="button"
            onClick={handlePayout}
            disabled={payoutLoading}
            className="px-4 py-2.5 bg-emerald-600 hover:bg-emerald-700 text-white font-bold text-sm rounded-xl shadow-sm transition-all flex items-center gap-2 disabled:opacity-50"
          >
            {payoutLoading
              ? "⏳ Đang đối soát..."
              : "💳 Rút tiền về Ví / Ngân hàng"}
          </button>
        </div>
      </div>

      {payoutSuccess && (
        <div className="p-4 bg-emerald-50 border border-emerald-200 rounded-xl text-emerald-800 text-sm font-semibold flex items-center gap-2">
          <span>
            ✓ Đã tạo lệnh rút 18.450.000₫ về tài khoản Vietcombank thành công!
            Mã đối soát: PO-98421
          </span>
        </div>
      )}

      {/* KPI Cards */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
        <div className="bg-white p-5 rounded-2xl border border-slate-100 shadow-sm">
          <div className="text-xs font-semibold text-slate-400 uppercase tracking-wider">
            Doanh thu hôm nay
          </div>
          <div className="text-2xl font-black text-slate-900 mt-2">
            12.450.000 ₫
          </div>
          <div className="text-xs font-medium text-emerald-600 mt-1">
            ↑ 18.2% so với hôm qua
          </div>
        </div>

        <div className="bg-white p-5 rounded-2xl border border-slate-100 shadow-sm">
          <div className="text-xs font-semibold text-slate-400 uppercase tracking-wider">
            Số dư Ví Người Bán
          </div>
          <div className="text-2xl font-black text-emerald-600 mt-2">
            18.450.000 ₫
          </div>
          <div className="text-xs font-medium text-slate-500 mt-1">
            Khả dụng để rút tiền ngay
          </div>
        </div>

        <div className="bg-white p-5 rounded-2xl border border-slate-100 shadow-sm">
          <div className="text-xs font-semibold text-slate-400 uppercase tracking-wider">
            Đơn hàng 7 ngày qua
          </div>
          <div className="text-2xl font-black text-blue-600 mt-2">148 đơn</div>
          <div className="text-xs font-medium text-blue-600 mt-1">
            Tỷ lệ hủy: 0.6% (Cực tốt)
          </div>
        </div>

        <div className="bg-white p-5 rounded-2xl border border-slate-100 shadow-sm">
          <div className="text-xs font-semibold text-slate-400 uppercase tracking-wider">
            Giá trị trung bình đơn (AOV)
          </div>
          <div className="text-2xl font-black text-purple-600 mt-2">
            1.820.000 ₫
          </div>
          <div className="text-xs font-medium text-purple-600 mt-1">
            Tỷ lệ chuyển đổi: 4.8%
          </div>
        </div>
      </div>

      {/* 7-day Revenue Chart Simulation & Performance */}
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        <div className="lg:col-span-2 bg-white p-6 rounded-2xl border border-slate-100 shadow-sm space-y-4">
          <div className="flex items-center justify-between">
            <h3 className="text-base font-bold text-slate-900">
              📈 Biểu đồ Doanh thu 7 ngày gần nhất
            </h3>
            <span className="text-xs font-semibold text-slate-400">
              Đơn vị: Triệu VND
            </span>
          </div>

          <div className="h-48 flex items-end justify-between gap-3 pt-6 px-2">
            {[
              { day: "T4", val: 8.2, height: "45%" },
              { day: "T5", val: 11.5, height: "62%" },
              { day: "T6", val: 14.8, height: "78%" },
              { day: "T7", val: 18.4, height: "95%" },
              { day: "CN", val: 16.2, height: "85%" },
              { day: "T2", val: 10.5, height: "55%" },
              { day: "Hôm nay", val: 12.4, height: "66%", current: true },
            ].map((col) => (
              <div
                key={col.day}
                className="flex-1 flex flex-col items-center gap-2 h-full justify-end group"
              >
                <span className="text-[11px] font-bold text-slate-600 group-hover:text-primary transition-colors">
                  {col.val}M
                </span>
                <div
                  style={{ height: col.height }}
                  className={`w-full rounded-t-lg transition-all duration-500 ${
                    col.current
                      ? "bg-gradient-to-t from-primary to-orange-400 shadow-md shadow-primary/30"
                      : "bg-slate-100 group-hover:bg-primary/40"
                  }`}
                />
                <span className="text-xs font-semibold text-slate-500">
                  {col.day}
                </span>
              </div>
            ))}
          </div>
        </div>

        {/* Top Products */}
        <div className="bg-white p-6 rounded-2xl border border-slate-100 shadow-sm space-y-4">
          <h3 className="text-base font-bold text-slate-900">
            🏆 Top 4 Sản Phẩm Bán Chạy
          </h3>
          <div className="space-y-3">
            {[
              {
                name: "iPhone 15 Pro Max 256GB",
                sales: 48,
                revenue: "1.439.520.000₫",
              },
              {
                name: "Tai nghe Sony WH-1000XM5",
                sales: 32,
                revenue: "220.800.000₫",
              },
              {
                name: "Bàn phím cơ Keychron K2 Pro",
                sales: 29,
                revenue: "57.710.000₫",
              },
              {
                name: "Chuột Logitech MX Master 3S",
                sales: 24,
                revenue: "59.760.000₫",
              },
            ].map((prod, idx) => (
              <div
                key={prod.name}
                className="flex items-center justify-between p-2.5 rounded-xl hover:bg-slate-50 transition-colors"
              >
                <div className="flex items-center gap-2.5 min-w-0">
                  <span className="w-5 h-5 rounded-full bg-slate-100 text-slate-600 font-bold text-xs flex items-center justify-center shrink-0">
                    {idx + 1}
                  </span>
                  <div className="truncate text-xs font-bold text-slate-800">
                    {prod.name}
                  </div>
                </div>
                <div className="text-right shrink-0">
                  <div className="text-xs font-black text-slate-900">
                    {prod.sales} đã bán
                  </div>
                </div>
              </div>
            ))}
          </div>
        </div>
      </div>
    </div>
  );
}
