"use client";

import Link from "next/link";
import { useState } from "react";

export default function SellerOrderDetailPage({
  params,
}: { params: { id: string } }) {
  const [status, setStatus] = useState("Đang đóng gói");
  const [trackingNumber, setTrackingNumber] = useState("SPX-VN-88492019");
  const [successMsg, setSuccessMsg] = useState("");

  const handlePrint = () => {
    window.print();
  };

  const handleShip = () => {
    setStatus("Đang vận chuyển");
    setSuccessMsg(
      "✓ Đã bàn giao cho đơn vị vận chuyển SPX Express thành công!",
    );
    setTimeout(() => setSuccessMsg(""), 4000);
  };

  return (
    <div className="space-y-6 max-w-4xl mx-auto">
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4 bg-white p-6 rounded-2xl border border-slate-100 shadow-sm print:hidden">
        <div>
          <div className="flex items-center gap-2">
            <Link
              href="/seller/orders"
              className="text-xs font-bold text-slate-400 hover:text-slate-600"
            >
              ← Quay lại danh sách
            </Link>
            <span className="text-slate-300">/</span>
            <span className="text-xs font-bold text-primary">
              Chi tiết đơn #{params.id}
            </span>
          </div>
          <h1 className="text-2xl font-black text-slate-900 tracking-tight mt-1">
            Xử Lý Đơn Hàng & In Phiếu Giao
          </h1>
        </div>

        <div className="flex items-center gap-3">
          <button
            type="button"
            onClick={handlePrint}
            className="px-4 py-2 bg-slate-100 hover:bg-slate-200 text-slate-800 font-bold text-xs rounded-xl transition-all flex items-center gap-1.5"
          >
            🖨️ In Phiếu Đóng Gói
          </button>
          {status === "Đang đóng gói" && (
            <button
              type="button"
              onClick={handleShip}
              className="px-4 py-2 bg-primary hover:bg-primary/90 text-white font-bold text-xs rounded-xl shadow-sm transition-all flex items-center gap-1.5"
            >
              🚚 Bàn Giao Vận Chuyển
            </button>
          )}
        </div>
      </div>

      {successMsg && (
        <div className="p-4 bg-emerald-50 border border-emerald-200 rounded-xl text-emerald-800 text-xs font-bold flex items-center gap-2 print:hidden">
          <span>{successMsg}</span>
        </div>
      )}

      {/* Printable Packing Slip */}
      <div className="bg-white p-8 rounded-2xl border border-slate-200 shadow-sm space-y-6 text-slate-900">
        <div className="border-b border-slate-200 pb-6 flex items-start justify-between">
          <div>
            <div className="text-xl font-black text-primary tracking-tight">
              MARKETPLACE LOGISTICS
            </div>
            <div className="text-xs text-slate-500 font-semibold mt-0.5">
              Đơn vị vận chuyển: SPX Express (Tiêu chuẩn)
            </div>
          </div>
          <div className="text-right">
            <div className="text-xs font-bold text-slate-400 uppercase">
              Mã Vận Đơn (Tracking)
            </div>
            <div className="text-lg font-black text-slate-900 tracking-wider font-mono">
              {trackingNumber}
            </div>
            <div className="inline-block mt-1 px-2 py-0.5 bg-blue-50 border border-blue-200 text-blue-700 text-[10px] font-bold rounded">
              {status}
            </div>
          </div>
        </div>

        {/* Sender and Receiver */}
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-6 bg-slate-50 p-4 rounded-xl border border-slate-100">
          <div>
            <div className="text-[11px] font-bold text-slate-400 uppercase">
              Người Gửi (Shop):
            </div>
            <div className="text-xs font-bold text-slate-900 mt-1">
              Apple Official Flagship Store
            </div>
            <div className="text-xs text-slate-600 mt-0.5">
              Kho hàng: Tầng 3, Tòa nhà Landmark 81, TP. Hồ Chí Minh
            </div>
            <div className="text-xs text-slate-600">Hotline: 1900 1599</div>
          </div>
          <div>
            <div className="text-[11px] font-bold text-slate-400 uppercase">
              Người Nhận:
            </div>
            <div className="text-xs font-bold text-slate-900 mt-1">
              Nguyễn Văn An
            </div>
            <div className="text-xs text-slate-600 mt-0.5">
              Số 18, Phố Tràng Tiền, Quận Hoàn Kiếm, Hà Nội
            </div>
            <div className="text-xs text-slate-600">SĐT: 0912 *** 888</div>
          </div>
        </div>

        {/* Items List */}
        <div>
          <div className="text-xs font-bold text-slate-900 uppercase tracking-wider mb-3">
            Danh Sách Hàng Hóa Đóng Gói:
          </div>
          <table className="w-full text-left text-xs border-collapse">
            <thead>
              <tr className="border-b border-slate-200 text-slate-400 font-semibold">
                <th className="py-2">STT</th>
                <th className="py-2">Tên sản phẩm</th>
                <th className="py-2">Phân loại</th>
                <th className="py-2 text-center">SL</th>
                <th className="py-2 text-right">Đơn giá</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100">
              <tr>
                <td className="py-3 font-semibold text-slate-500">1</td>
                <td className="py-3 font-bold text-slate-900">
                  iPhone 15 Pro Max 256GB Titan Tự Nhiên
                </td>
                <td className="py-3 text-slate-600">Titan Tự Nhiên, 256GB</td>
                <td className="py-3 text-center font-bold text-slate-900">1</td>
                <td className="py-3 text-right font-bold text-slate-900">
                  29.990.000 ₫
                </td>
              </tr>
            </tbody>
          </table>
        </div>

        <div className="border-t border-slate-200 pt-4 flex items-center justify-between">
          <div className="text-xs text-slate-500">
            * Khách hàng được quyền kiểm tra hàng trước khi nhận. Vui lòng quay
            video clip khi mở gói hàng.
          </div>
          <div className="text-right">
            <span className="text-xs text-slate-500">
              Tổng tiền thu hộ (COD):{" "}
            </span>
            <span className="text-base font-black text-primary">
              0 ₫ (Đã thanh toán online)
            </span>
          </div>
        </div>
      </div>
    </div>
  );
}
