"use client";

import { type FormEvent, useState } from "react";

import { Alert } from "@/components/ui/Alert";
import { Button } from "@/components/ui/Button";
import { Card } from "@/components/ui/Card";
import { Input } from "@/components/ui/Input";
import { Link } from "@/components/ui/Link";
import { Modal } from "@/components/ui/Modal";
import type {
  CreatePlatformVoucherInput,
  VoucherView,
} from "@/lib/gateway/vouchers";
import { usePending } from "@/lib/usePending";

import { VoucherDetails } from "./VoucherDetails";
import { createPlatformVoucherAction } from "./actions";
import {
  EMPTY_VOUCHER_FORM,
  type VoucherField,
  type VoucherFieldErrors,
  type VoucherFormValues,
  validateVoucherForm,
} from "./voucherForm";
import {
  describeDiscount,
  describeMinSpend,
  describeUsage,
  formatVnDateTime,
  voucherHref,
} from "./voucherFormat";

const selectClass =
  "min-h-touch w-full rounded-xl border border-border-strong bg-surface-card px-4 py-3 text-base text-text-primary";

function Summary({ input }: { input: CreatePlatformVoucherInput }) {
  const rows: [string, string][] = [
    ["Mã voucher", input.code],
    ["Phạm vi", "Toàn sàn"],
    ["Mức giảm", describeDiscount(input)],
    ["Đơn tối thiểu", describeMinSpend(input.minSpend)],
    ["Lượt dùng tối đa", describeUsage({ used: 0, quota: input.quota })],
    ["Bắt đầu", formatVnDateTime(input.startsAt.toISOString())],
    ["Kết thúc", formatVnDateTime(input.endsAt.toISOString())],
  ];
  return (
    <dl
      data-testid="voucher-confirm-summary"
      className="grid grid-cols-1 gap-x-6 gap-y-3 text-sm sm:grid-cols-2"
    >
      {rows.map(([label, value]) => (
        <div key={label} className="flex flex-col">
          <dt className="text-text-secondary">{label}</dt>
          <dd className="font-medium text-text-primary">{value}</dd>
        </div>
      ))}
    </dl>
  );
}

export function CreateVoucherForm() {
  const [values, setValues] = useState<VoucherFormValues>(EMPTY_VOUCHER_FORM);
  const [errors, setErrors] = useState<VoucherFieldErrors>({});
  const [review, setReview] = useState<CreatePlatformVoucherInput | null>(null);
  const [acknowledged, setAcknowledged] = useState(false);
  const [error, setError] = useState("");
  const [created, setCreated] = useState<VoucherView | null>(null);
  const [createdAt, setCreatedAt] = useState<Date>(() => new Date());
  const { pending, run } = usePending();

  const set = (field: VoucherField) => (value: string) =>
    setValues((v) => ({ ...v, [field]: value }));

  function onReview(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    setError("");
    const checked = validateVoucherForm(values, new Date());
    if (!checked.ok) {
      setErrors(checked.errors);
      return;
    }
    setErrors({});
    setAcknowledged(false);
    setReview(checked.input);
  }

  function closeReview() {
    if (pending) return;
    setError("");
    setReview(null);
  }

  async function onConfirm() {
    if (pending || !acknowledged) return;
    setError("");
    const res = await run(() => createPlatformVoucherAction(values));
    if (res.ok) {
      setReview(null);
      setCreatedAt(new Date());
      setCreated(res.voucher);
      return;
    }
    if (res.fieldErrors) {
      setErrors(res.fieldErrors);
      setReview(null);
      return;
    }
    setError(res.error);
  }

  function startOver() {
    setCreated(null);
    setValues(EMPTY_VOUCHER_FORM);
    setErrors({});
  }

  if (created) {
    return (
      <div className="flex flex-col gap-6" data-testid="voucher-created">
        <Alert
          type="success"
          title={`Đã tạo voucher sàn ${created.code}`}
          description="Thông tin dưới đây là dữ liệu dịch vụ khuyến mãi trả về sau khi tạo."
        />
        <Card padded>
          <VoucherDetails voucher={created} now={createdAt} />
        </Card>
        <div className="flex flex-col gap-3 sm:flex-row sm:flex-wrap">
          <Link
            href={voucherHref(created.code)}
            variant="primary"
            size="md"
            fullWidth="mobile"
          >
            Xem chi tiết
          </Link>
          <Link
            href="/admin/vouchers"
            variant="outline"
            size="md"
            fullWidth="mobile"
          >
            Về danh sách
          </Link>
          <Button variant="secondary" fullWidth="mobile" onClick={startOver}>
            Tạo voucher khác
          </Button>
        </div>
      </div>
    );
  }

  const percent = values.discountType === "percent";
  const reviewOpen = review !== null;

  return (
    <>
      <Card padded>
        <form
          noValidate
          data-testid="voucher-create-form"
          onSubmit={onReview}
          className="grid grid-cols-1 gap-5 md:grid-cols-2 md:gap-6"
        >
          <Input
            label="Mã voucher"
            name="code"
            required
            autoComplete="off"
            value={values.code}
            onChange={(e) => set("code")(e.target.value.toUpperCase())}
            helperText="3 đến 32 ký tự: chữ cái không dấu, số, gạch ngang hoặc gạch dưới."
            error={errors.code}
          />
          <div className="flex flex-col gap-1">
            <label
              htmlFor="voucher-discount-type"
              className="text-sm font-semibold text-text-primary"
            >
              Loại giảm giá
            </label>
            <select
              id="voucher-discount-type"
              name="discountType"
              value={values.discountType}
              onChange={(e) => set("discountType")(e.target.value)}
              className={selectClass}
              aria-invalid={errors.discountType ? true : undefined}
            >
              <option value="percent">Theo phần trăm</option>
              <option value="fixed">Số tiền cố định</option>
            </select>
            {errors.discountType ? (
              <p className="text-sm text-danger">{errors.discountType}</p>
            ) : null}
          </div>
          <Input
            label={percent ? "Phần trăm giảm" : "Số tiền giảm (đồng)"}
            name="discountValue"
            required
            inputMode="numeric"
            value={values.discountValue}
            onChange={(e) => set("discountValue")(e.target.value)}
            error={errors.discountValue}
          />
          {percent ? (
            <Input
              label="Giảm tối đa (đồng)"
              name="maxDiscount"
              inputMode="numeric"
              value={values.maxDiscount}
              onChange={(e) => set("maxDiscount")(e.target.value)}
              helperText="0 là không giới hạn mức giảm."
              error={errors.maxDiscount}
            />
          ) : null}
          <Input
            label="Đơn tối thiểu (đồng)"
            name="minSpend"
            inputMode="numeric"
            value={values.minSpend}
            onChange={(e) => set("minSpend")(e.target.value)}
            helperText="0 là không yêu cầu."
            error={errors.minSpend}
          />
          <Input
            label="Lượt dùng tối đa"
            name="quota"
            inputMode="numeric"
            value={values.quota}
            onChange={(e) => set("quota")(e.target.value)}
            helperText="0 là không giới hạn."
            error={errors.quota}
          />
          <Input
            label="Bắt đầu (giờ Việt Nam)"
            name="startsAt"
            type="datetime-local"
            required
            value={values.startsAt}
            onChange={(e) => set("startsAt")(e.target.value)}
            error={errors.startsAt}
          />
          <Input
            label="Kết thúc (giờ Việt Nam)"
            name="endsAt"
            type="datetime-local"
            required
            value={values.endsAt}
            onChange={(e) => set("endsAt")(e.target.value)}
            error={errors.endsAt}
          />
          <div className="flex flex-col gap-3 border-t border-border-subtle pt-6 sm:flex-row md:col-span-2">
            <Button type="submit" fullWidth="mobile">
              Xem lại trước khi tạo
            </Button>
            <Link
              href="/admin/vouchers"
              variant="outline"
              size="md"
              fullWidth="mobile"
            >
              Huỷ
            </Link>
          </div>
        </form>
      </Card>

      <Modal
        isOpen={reviewOpen}
        onClose={closeReview}
        title="Xác nhận tạo voucher sàn"
        description="Voucher này áp dụng cho mọi người mua trên toàn sàn. Hiện chưa có cách sửa, ngừng hay xoá voucher sau khi tạo."
        footer={
          <>
            <Button
              variant="secondary"
              onClick={closeReview}
              disabled={pending}
            >
              Quay lại sửa
            </Button>
            <Button
              onClick={onConfirm}
              isLoading={pending}
              disabled={pending || !acknowledged}
            >
              Tạo voucher
            </Button>
          </>
        }
      >
        <div className="flex flex-col gap-4">
          {review ? <Summary input={review} /> : null}
          <label className="flex min-h-touch items-center gap-3 text-sm text-text-primary">
            <input
              type="checkbox"
              data-testid="voucher-confirm-ack"
              checked={acknowledged}
              onChange={(e) => setAcknowledged(e.target.checked)}
              className="h-5 w-5"
            />
            Tôi đã kiểm tra và hiểu voucher này áp dụng cho toàn sàn
          </label>
          {error ? (
            <div data-testid="voucher-create-error">
              <Alert type="error" description={error} />
            </div>
          ) : null}
        </div>
      </Modal>
    </>
  );
}
