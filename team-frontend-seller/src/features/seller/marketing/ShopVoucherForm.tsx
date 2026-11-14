"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";

import { Alert } from "@/components/ui/Alert";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { Select } from "@/components/ui/Select";
import { DiscountType } from "@/generated/platform/promotion/v1/promotion_pb.js";
import { createShopVoucherAction } from "../actions";
import { usePending } from "../usePending";

type Field = "code" | "value" | "quota" | "start" | "end";

const EMPTY = {
  code: "",
  type: String(DiscountType.PERCENT),
  value: "",
  minSpend: "",
  maxDiscount: "",
  quota: "",
  start: "",
  end: "",
};

export function ShopVoucherForm() {
  const router = useRouter();
  const [f, setF] = useState(EMPTY);
  const [errors, setErrors] = useState<Partial<Record<Field, string>>>({});
  const [serverError, setServerError] = useState("");
  const { pending, run } = usePending();
  const set = (k: keyof typeof EMPTY) => (e: { target: { value: string } }) =>
    setF((p) => ({ ...p, [k]: e.target.value }));

  function validate() {
    const errs: Partial<Record<Field, string>> = {};
    const value = Number(f.value);
    const quota = Number(f.quota);
    const start = new Date(f.start);
    const end = new Date(f.end);
    if (!f.code.trim()) errs.code = "Vui lòng nhập mã voucher";
    if (!(value > 0)) errs.value = "Giá trị giảm phải lớn hơn 0";
    else if (f.type === String(DiscountType.PERCENT) && value > 100) {
      errs.value = "Giảm theo phần trăm tối đa 100";
    }
    if (!Number.isInteger(quota) || quota < 1) {
      errs.quota = "Số lượt dùng phải từ 1 trở lên";
    }
    if (Number.isNaN(start.getTime())) errs.start = "Chọn thời gian bắt đầu";
    if (Number.isNaN(end.getTime())) errs.end = "Chọn thời gian kết thúc";
    else if (!errs.start && end <= start) {
      errs.end = "Thời gian kết thúc phải sau thời gian bắt đầu";
    }
    return errs;
  }

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    if (pending) return;
    const errs = validate();
    setErrors(errs);
    setServerError("");
    if (Object.keys(errs).length > 0) return;
    const res = await run(() =>
      createShopVoucherAction({
        code: f.code.trim(),
        discountType: Number(f.type) as DiscountType,
        discountValue: Number(f.value),
        minSpend: Number(f.minSpend) || 0,
        maxDiscount: Number(f.maxDiscount) || 0,
        quota: Number(f.quota),
        startsAt: new Date(f.start),
        endsAt: new Date(f.end),
      }),
    );
    if (res.ok) {
      setF(EMPTY);
      router.refresh();
    } else {
      setServerError(res.error);
    }
  }

  return (
    <form onSubmit={submit} noValidate className="space-y-4">
      <fieldset disabled={pending} className="m-0 space-y-4 border-0 p-0">
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <Input
            label="Mã voucher"
            required
            value={f.code}
            error={errors.code}
            onChange={set("code")}
          />
          <Select
            label="Loại giảm giá"
            value={f.type}
            onChange={set("type")}
            options={[
              { value: String(DiscountType.PERCENT), label: "Phần trăm (%)" },
              { value: String(DiscountType.FIXED), label: "Số tiền (₫)" },
            ]}
          />
          <Input
            label="Giá trị giảm"
            required
            type="number"
            min={0}
            value={f.value}
            error={errors.value}
            onChange={set("value")}
          />
          <Input
            label="Số lượt dùng"
            required
            type="number"
            min={1}
            value={f.quota}
            error={errors.quota}
            onChange={set("quota")}
          />
          <Input
            label="Chi tiêu tối thiểu (₫)"
            type="number"
            min={0}
            value={f.minSpend}
            onChange={set("minSpend")}
          />
          <Input
            label="Giảm tối đa (₫)"
            type="number"
            min={0}
            value={f.maxDiscount}
            onChange={set("maxDiscount")}
          />
          <Input
            label="Bắt đầu"
            required
            type="datetime-local"
            value={f.start}
            error={errors.start}
            onChange={set("start")}
          />
          <Input
            label="Kết thúc"
            required
            type="datetime-local"
            value={f.end}
            error={errors.end}
            onChange={set("end")}
          />
        </div>
      </fieldset>
      {Object.keys(errors).length > 0 && (
        <p role="alert" className="text-sm text-danger">
          Vui lòng kiểm tra lại các trường được đánh dấu.
        </p>
      )}
      {serverError && <Alert type="error" description={serverError} />}
      <Button type="submit" isLoading={pending}>
        Tạo voucher
      </Button>
    </form>
  );
}
