"use client";

import { useState } from "react";

import { Alert } from "@/components/ui/Alert";
import { Button } from "@/components/ui/Button";
import { Card, CardHeader, CardTitle } from "@/components/ui/Card";
import { Empty } from "@/components/ui/Empty";
import { FormItem } from "@/components/ui/FormItem";
import { Input } from "@/components/ui/Input";
import { Select } from "@/components/ui/Select";
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
      if (!res.ok || !res.data) {
        const message = res.ok ? res.message : res.error;
        setFormError(message);
        toast.error(message);
        return;
      }
      const created = res.data;
      setVouchers((prev) => [created, ...prev]);
      setCode("");
      toast.success(res.message);
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="space-y-6">
      <Card>
        <CardHeader>
          <CardTitle>Tạo Voucher (Người bán / Sàn)</CardTitle>
        </CardHeader>

        <form
          onSubmit={handleSubmit}
          aria-label="Create voucher"
          className="grid grid-cols-1 gap-4 p-5 sm:grid-cols-2"
        >
          <FormItem label="Mã Voucher">
            <Input
              type="text"
              name="code"
              value={code}
              onChange={(e) => setCode(e.target.value)}
              placeholder="VD: SALE50"
              className="uppercase"
            />
          </FormItem>

          <FormItem label="Phạm vi">
            <Select
              name="scope"
              value={String(scope)}
              onChange={(e) => setScope(Number(e.target.value) as VoucherScope)}
              options={[
                { value: String(VoucherScope.SHOP), label: "Shop" },
                { value: String(VoucherScope.PLATFORM), label: "Toàn sàn" },
              ]}
            />
          </FormItem>

          <FormItem label="Loại giảm giá">
            <Select
              name="discount_type"
              value={String(discountType)}
              onChange={(e) =>
                setDiscountType(Number(e.target.value) as DiscountType)
              }
              options={[
                {
                  value: String(DiscountType.PERCENT),
                  label: "Giảm % (1-100)",
                },
                { value: String(DiscountType.FIXED), label: "Giảm tiền (VND)" },
              ]}
            />
          </FormItem>

          <FormItem label="Giá trị giảm">
            <Input
              type="number"
              name="discount_value"
              min="0"
              value={discountValue}
              onChange={(e) => setDiscountValue(e.target.value)}
            />
          </FormItem>

          <FormItem label="Đơn tối thiểu (VND)">
            <Input
              type="number"
              name="min_spend"
              min="0"
              value={minSpend}
              onChange={(e) => setMinSpend(e.target.value)}
            />
          </FormItem>

          <FormItem label="Giảm tối đa (VND, 0 = không giới hạn)">
            <Input
              type="number"
              name="max_discount"
              min="0"
              value={maxDiscount}
              onChange={(e) => setMaxDiscount(e.target.value)}
            />
          </FormItem>

          <FormItem label="Số lượng (quota)">
            <Input
              type="number"
              name="quota"
              min="0"
              value={quota}
              onChange={(e) => setQuota(e.target.value)}
            />
          </FormItem>

          <div className="flex items-end">
            <Button
              type="submit"
              aria-label="Create voucher"
              isLoading={submitting}
              className="w-full"
            >
              Tạo Voucher
            </Button>
          </div>

          {formError && (
            <div className="sm:col-span-2">
              <Alert type="error" description={formError} />
            </div>
          )}
        </form>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Danh sách Voucher ({vouchers.length})</CardTitle>
        </CardHeader>
        {vouchers.length === 0 ? (
          <Empty description="Chưa có voucher nào. Hãy tạo voucher đầu tiên." />
        ) : (
          <div className="overflow-x-auto p-5">
            <table className="w-full text-left text-sm">
              <thead>
                <tr className="border-b border-border-subtle text-text-secondary">
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
                    className="border-b border-border-subtle text-text-primary"
                  >
                    <td className="py-2 pr-3 font-semibold text-action-primary">
                      {v.code}
                    </td>
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
      </Card>
    </div>
  );
}
