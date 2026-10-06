"use client";

import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { useState } from "react";

import { Button } from "@/components/ui/Button";
import { Card } from "@/components/ui/Card";
import { Empty } from "@/components/ui/Empty";
import { FormItem } from "@/components/ui/FormItem";
import { Input } from "@/components/ui/Input";
import { Modal } from "@/components/ui/Modal";
import { RadioGroup } from "@/components/ui/Radio";
import { Tag } from "@/components/ui/Tag";
import { useToast } from "@/components/ui/ToastProvider";
import { formatPrice } from "@/components/ui/format";
import { DiscountType } from "@/generated/platform/promotion/v1/promotion_pb.js";
import { trackEcommerce } from "@/lib/analytics";
import type { ViewVoucher } from "@/lib/gateway/promotion";
import { previewVoucherAction } from "./actions";

function describeVoucher(v: ViewVoucher): string {
  const off =
    v.discountType === DiscountType.PERCENT
      ? `Giảm ${v.discountValue}%`
      : `Giảm ${formatPrice(v.discountValue)}`;
  const cap =
    v.discountType === DiscountType.PERCENT && v.maxDiscount > 0
      ? `, tối đa ${formatPrice(v.maxDiscount)}`
      : "";
  const min = v.minSpend > 0 ? ` · Đơn từ ${formatPrice(v.minSpend)}` : "";
  return `${off}${cap}${min}`;
}

/**
 * Voucher selector row + modal. The applied code lives in `?voucher=`; the
 * discount is previewed by the server (previewVoucherAction), never computed
 * here. An invalid code leaves any applied voucher unchanged.
 */
export function VoucherModal({
  vouchers,
  subtotal,
  sellerId,
  appliedCode,
  appliedDiscount,
}: {
  vouchers: ViewVoucher[];
  subtotal: number;
  sellerId: string;
  appliedCode?: string;
  appliedDiscount: number;
}) {
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const toast = useToast();
  const [open, setOpen] = useState(false);
  const [code, setCode] = useState("");
  const [error, setError] = useState("");
  const [applying, setApplying] = useState(false);

  function setVoucherParam(next: string | null) {
    const params = new URLSearchParams(searchParams.toString());
    if (next) params.set("voucher", next);
    else params.delete("voucher");
    const query = params.toString();
    router.replace(query ? `${pathname}?${query}` : pathname);
  }

  async function handleApply() {
    const trimmed = code.trim();
    if (!trimmed) return;
    setApplying(true);
    setError("");
    try {
      const res = await previewVoucherAction(trimmed, subtotal, sellerId);
      trackEcommerce("apply_promotion", {
        coupon: trimmed,
        value: res.discountAmount || 0,
        properties: { valid: String(res.valid) },
      });
      if (!res.valid) {
        const msg = res.reason || "Mã giảm giá không hợp lệ.";
        setError(msg);
        toast.error(msg);
        return;
      }
      toast.success(
        `Đã áp dụng mã ${trimmed}: -${formatPrice(res.discountAmount)}`,
      );
      setOpen(false);
      setCode("");
      setVoucherParam(trimmed);
    } catch {
      const msg = "Không kiểm tra được mã giảm giá. Vui lòng thử lại.";
      setError(msg);
      toast.error(msg);
    } finally {
      setApplying(false);
    }
  }

  function handleRemove() {
    setVoucherParam(null);
    toast.info("Đã hủy áp dụng mã giảm giá.");
  }

  return (
    <>
      <Card>
        <div className="flex flex-wrap items-center justify-between gap-3 p-4">
          <div className="flex flex-wrap items-center gap-2">
            <span className="text-sm font-semibold text-text-primary">
              Mã giảm giá
            </span>
            {appliedCode && (
              <>
                <Tag
                  color="success"
                  closable
                  closeLabel={`Bỏ mã ${appliedCode}`}
                  onClose={handleRemove}
                >
                  {appliedCode}
                </Tag>
                {appliedDiscount > 0 && (
                  <span className="text-xs text-text-secondary">
                    -{formatPrice(appliedDiscount)}
                  </span>
                )}
              </>
            )}
          </div>
          <Button
            variant="outline"
            size="sm"
            onClick={() => {
              setError("");
              setOpen(true);
            }}
          >
            {appliedCode ? "Đổi mã" : "Chọn hoặc nhập mã"}
          </Button>
        </div>
      </Card>

      <Modal
        isOpen={open}
        onClose={() => setOpen(false)}
        title="Chọn mã giảm giá"
      >
        <div className="space-y-5">
          <form
            className="flex items-start gap-2"
            onSubmit={(e) => {
              e.preventDefault();
              void handleApply();
            }}
          >
            <div className="flex-1">
              <FormItem
                label="Nhập mã giảm giá"
                status={error ? "error" : undefined}
                help={error || undefined}
              >
                <Input
                  name="voucher_code"
                  value={code}
                  placeholder="Ví dụ: SAVE10"
                  autoComplete="off"
                  onChange={(e) => {
                    setCode(e.target.value);
                    if (error) setError("");
                  }}
                />
              </FormItem>
            </div>
            <Button
              type="submit"
              className="mt-6"
              isLoading={applying}
              disabled={!code.trim()}
            >
              Áp dụng
            </Button>
          </form>

          {vouchers.length === 0 ? (
            <Empty description="Chưa có voucher khả dụng" />
          ) : (
            <RadioGroup
              legend="Voucher khả dụng"
              name="available_voucher"
              value={code}
              onChange={setCode}
              options={vouchers.map((v) => ({
                value: v.code,
                label: v.code,
                description: describeVoucher(v),
              }))}
            />
          )}
        </div>
      </Modal>
    </>
  );
}
