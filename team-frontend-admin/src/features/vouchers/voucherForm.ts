import type { CreatePlatformVoucherInput } from "@/lib/gateway/vouchers";

export interface VoucherFormValues {
  code: string;
  discountType: string;
  discountValue: string;
  minSpend: string;
  maxDiscount: string;
  quota: string;
  startsAt: string;
  endsAt: string;
}

export type VoucherField = keyof VoucherFormValues;

export type VoucherFieldErrors = Partial<Record<VoucherField, string>>;

export type VoucherFormValidation =
  | { ok: true; input: CreatePlatformVoucherInput }
  | { ok: false; errors: VoucherFieldErrors };

export const EMPTY_VOUCHER_FORM: VoucherFormValues = {
  code: "",
  discountType: "percent",
  discountValue: "",
  minSpend: "0",
  maxDiscount: "0",
  quota: "0",
  startsAt: "",
  endsAt: "",
};

export const CODE_PATTERN = /^[A-Z0-9_-]{3,32}$/;

const DATETIME_LOCAL = /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}$/;

export function normalizeCode(raw: string): string {
  return raw.trim().toUpperCase();
}

export function parseVnDateTime(raw: string): Date | null {
  if (!DATETIME_LOCAL.test(raw)) return null;
  const d = new Date(`${raw}:00+07:00`);
  return Number.isNaN(d.getTime()) ? null : d;
}

function parseAmount(raw: string, emptyAs?: number): number | null {
  const t = raw.trim();
  if (t === "" && emptyAs !== undefined) return emptyAs;
  if (!/^\d+$/.test(t)) return null;
  const n = Number(t);
  return Number.isSafeInteger(n) ? n : null;
}

export function validateVoucherForm(
  values: VoucherFormValues,
  now: Date,
): VoucherFormValidation {
  const errors: VoucherFieldErrors = {};

  const code = normalizeCode(values.code);
  if (!CODE_PATTERN.test(code)) {
    errors.code =
      "Mã gồm 3 đến 32 ký tự: chữ cái không dấu, số, gạch ngang hoặc gạch dưới.";
  }

  const type = values.discountType;
  if (type !== "percent" && type !== "fixed") {
    errors.discountType = "Chọn loại giảm giá.";
  }

  const value = parseAmount(values.discountValue);
  if (type === "percent") {
    if (value === null || value < 1 || value > 100) {
      errors.discountValue = "Phần trăm giảm là số nguyên từ 1 đến 100.";
    }
  } else if (value === null || value < 1) {
    errors.discountValue = "Số tiền giảm là số nguyên dương (đồng).";
  }

  const minSpend = parseAmount(values.minSpend, 0);
  if (minSpend === null) {
    errors.minSpend = "Đơn tối thiểu là số nguyên không âm (đồng).";
  }

  const maxDiscount =
    type === "percent" ? parseAmount(values.maxDiscount, 0) : 0;
  if (maxDiscount === null) {
    errors.maxDiscount = "Mức giảm tối đa là số nguyên không âm (đồng).";
  }

  const quota = parseAmount(values.quota, 0);
  if (quota === null) {
    errors.quota = "Số lượt dùng là số nguyên không âm.";
  }

  const startsAt = parseVnDateTime(values.startsAt);
  if (!startsAt) errors.startsAt = "Chọn thời điểm bắt đầu.";
  const endsAt = parseVnDateTime(values.endsAt);
  if (!endsAt) {
    errors.endsAt = "Chọn thời điểm kết thúc.";
  } else if (endsAt.getTime() <= now.getTime()) {
    errors.endsAt = "Thời điểm kết thúc phải ở tương lai.";
  } else if (startsAt && endsAt.getTime() <= startsAt.getTime()) {
    errors.endsAt = "Thời điểm kết thúc phải sau thời điểm bắt đầu.";
  }

  if (
    Object.keys(errors).length > 0 ||
    value === null ||
    minSpend === null ||
    maxDiscount === null ||
    quota === null ||
    !startsAt ||
    !endsAt
  ) {
    return { ok: false, errors };
  }

  return {
    ok: true,
    input: {
      code,
      discountType: type as "percent" | "fixed",
      discountValue: value,
      minSpend,
      maxDiscount,
      quota,
      startsAt,
      endsAt,
    },
  };
}
