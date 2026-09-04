"use client";

import { useState } from "react";

import { useToast } from "@/components/ui/ToastProvider";
import {
  DiscountType,
  VoucherScope,
} from "@/generated/platform/promotion/v1/promotion_pb.js";
import type { ViewVoucher } from "@/lib/gateway/promotion";
import { createVoucherAction } from "./actions";

/**
 * Seller/admin voucher console: create a voucher and list existing ones. All
 * writes/reads go through team-promotion via the gateway (server action →
 * gateway module). Kept intentionally basic — enough to seed/verify in e2e.
 */
export function VoucherManager({
  initialVouchers,
}: {
  initialVouchers: ViewVoucher[];
}) {
  const toast = useToast();
  const [vouchers, setVouchers] = useState<ViewVoucher[]>(initialVouchers);
  const [code, setCode] = useState("");
  const [discountType, setDiscountType] = useState<DiscountType>(
    DiscountType.PERCENT,
  );
  const [discountValue, setDiscountValue] = useState("10");
  const [minSpend, setMinSpend] = useState("0");
  const [maxDiscount, setMaxDiscount] = useState("0");
  const [quota, setQuota] = useState("100");
  const [scope, setScope] = useState<VoucherScope>(VoucherScope.SHOP);
  const [submitting, setSubmitting] = useState(false);
  const [formError, setFormError] = useState("");

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setFormError("");
    if (!code.trim()) {
      setFormError("Vui lòng nhập mã voucher.");
      return;
    }
    setSubmitting(true);
    try {
      const res = await createVoucherAction({
        code: code.trim().toUpperCase(),
        scope,
        discountType,
        discountValue: Number(discountValue) || 0,
        minSpend: Number(minSpend) || 0,
        maxDiscount: Number(maxDiscount) || 0,
        quota: Number(quota) || 0,
      });
      if (!res.ok || !res.voucher) {
        setFormError(res.message);
        toast.error(res.message);
        return;
      }
      setVouchers((prev) => [res.voucher as ViewVoucher, ...prev]);
      setCode("");
      toast.success(res.message);
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="space-y-6">
      <div className="rounded-xl border border-orange-200 bg-white p-5 shadow-xs">
        <h2 className="border-b pb-3 text-base font-bold text-gray-900 flex items-center gap-2">
          <span>🎟️</span>
          <span>Tạo Voucher (Người bán / Sàn)</span>
        </h2>

        <form
          onSubmit={handleSubmit}
          aria-label="Create voucher"
          className="mt-4 grid grid-cols-1 gap-3 sm:grid-cols-2"
        >
          <label className="flex flex-col gap-1 text-xs font-medium text-gray-600">
            Mã Voucher
            <input
              type="text"
              name="code"
              value={code}
              onChange={(e) => setCode(e.target.value)}
              placeholder="VD: SALE50"
              className="rounded-lg border border-gray-300 px-3 py-1.5 text-sm text-gray-800 uppercase focus:border-brand focus:outline-hidden"
            />
          </label>

          <label className="flex flex-col gap-1 text-xs font-medium text-gray-600">
            Phạm vi
            <select
              name="scope"
              value={scope}
              onChange={(e) => setScope(Number(e.target.value) as VoucherScope)}
              className="rounded-lg border border-gray-300 px-3 py-1.5 text-sm text-gray-800 focus:border-brand focus:outline-hidden"
            >
              <option value={VoucherScope.SHOP}>Shop</option>
              <option value={VoucherScope.PLATFORM}>Toàn sàn</option>
            </select>
          </label>

          <label className="flex flex-col gap-1 text-xs font-medium text-gray-600">
            Loại giảm giá
            <select
              name="discount_type"
              value={discountType}
              onChange={(e) =>
                setDiscountType(Number(e.target.value) as DiscountType)
              }
              className="rounded-lg border border-gray-300 px-3 py-1.5 text-sm text-gray-800 focus:border-brand focus:outline-hidden"
            >
              <option value={DiscountType.PERCENT}>Giảm % (1-100)</option>
              <option value={DiscountType.FIXED}>Giảm tiền (VND)</option>
            </select>
          </label>

          <label className="flex flex-col gap-1 text-xs font-medium text-gray-600">
            Giá trị giảm
            <input
              type="number"
              name="discount_value"
              min="0"
              value={discountValue}
              onChange={(e) => setDiscountValue(e.target.value)}
              className="rounded-lg border border-gray-300 px-3 py-1.5 text-sm text-gray-800 focus:border-brand focus:outline-hidden"
            />
          </label>

          <label className="flex flex-col gap-1 text-xs font-medium text-gray-600">
            Đơn tối thiểu (VND)
            <input
              type="number"
              name="min_spend"
              min="0"
              value={minSpend}
              onChange={(e) => setMinSpend(e.target.value)}
              className="rounded-lg border border-gray-300 px-3 py-1.5 text-sm text-gray-800 focus:border-brand focus:outline-hidden"
            />
          </label>

          <label className="flex flex-col gap-1 text-xs font-medium text-gray-600">
            Giảm tối đa (VND, 0 = không giới hạn)
            <input
              type="number"
              name="max_discount"
              min="0"
              value={maxDiscount}
              onChange={(e) => setMaxDiscount(e.target.value)}
              className="rounded-lg border border-gray-300 px-3 py-1.5 text-sm text-gray-800 focus:border-brand focus:outline-hidden"
            />
          </label>

          <label className="flex flex-col gap-1 text-xs font-medium text-gray-600">
            Số lượng (quota)
            <input
              type="number"
              name="quota"
              min="0"
              value={quota}
              onChange={(e) => setQuota(e.target.value)}
              className="rounded-lg border border-gray-300 px-3 py-1.5 text-sm text-gray-800 focus:border-brand focus:outline-hidden"
            />
          </label>

          <div className="flex items-end">
            <button
              type="submit"
              aria-label="Create voucher"
              disabled={submitting}
              className="w-full rounded-lg bg-brand px-4 py-2 text-sm font-bold text-white shadow-sm hover:bg-brand-dark disabled:opacity-50"
            >
              {submitting ? "Đang tạo..." : "Tạo Voucher"}
            </button>
          </div>
        </form>
        {formError && (
          <p className="mt-2 text-xs text-red-500" role="alert">
            {formError}
          </p>
        )}
      </div>

      <div className="rounded-xl border bg-white p-5 shadow-xs">
        <h2 className="border-b pb-3 text-base font-bold text-gray-900">
          Danh sách Voucher ({vouchers.length})
        </h2>
        {vouchers.length === 0 ? (
          <p className="py-6 text-center text-sm text-gray-400">
            Chưa có voucher nào. Hãy tạo voucher đầu tiên.
          </p>
        ) : (
          <div className="mt-3 overflow-x-auto">
            <table className="w-full text-left text-xs">
              <thead>
                <tr className="border-b text-gray-400">
                  <th className="py-2 pr-3 font-medium">Mã</th>
                  <th className="py-2 pr-3 font-medium">Loại</th>
                  <th className="py-2 pr-3 font-medium">Giá trị</th>
                  <th className="py-2 pr-3 font-medium">Đơn tối thiểu</th>
                  <th className="py-2 pr-3 font-medium">Phạm vi</th>
                  <th className="py-2 pr-3 font-medium">Đã dùng / Quota</th>
                </tr>
              </thead>
              <tbody>
                {vouchers.map((v) => (
                  <tr
                    key={v.id || v.code}
                    data-testid="voucher-row"
                    data-code={v.code}
                    className="border-b border-gray-50 text-gray-700"
                  >
                    <td className="py-2 pr-3 font-bold text-brand">{v.code}</td>
                    <td className="py-2 pr-3">{v.discountTypeText}</td>
                    <td className="py-2 pr-3">
                      {v.discountType === DiscountType.PERCENT
                        ? `${v.discountValue}%`
                        : `${v.discountValue.toLocaleString("vi-VN")} VND`}
                    </td>
                    <td className="py-2 pr-3">
                      {v.minSpend.toLocaleString("vi-VN")} VND
                    </td>
                    <td className="py-2 pr-3">{v.scopeText}</td>
                    <td className="py-2 pr-3">
                      {v.used} / {v.quota === 0 ? "∞" : v.quota}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  );
}
