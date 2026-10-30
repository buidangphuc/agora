import { formatPrice } from "@/components/ui/format";
import type { VoucherView } from "@/lib/gateway/vouchers";

export type VoucherState = "upcoming" | "active" | "ended" | "exhausted";

export const STATE_LABEL: Record<VoucherState, string> = {
  upcoming: "Chưa bắt đầu",
  active: "Đang chạy",
  ended: "Đã kết thúc",
  exhausted: "Hết lượt",
};

export const SCOPE_LABEL: Record<VoucherView["scope"], string> = {
  platform: "Toàn sàn",
  shop: "Cửa hàng",
  unspecified: "Không xác định",
};

const VN_OFFSET_MS = 7 * 60 * 60 * 1000;
const pad = (n: number) => String(n).padStart(2, "0");

export function formatVnDateTime(iso: string): string {
  if (!iso) return "Không đặt";
  const t = new Date(iso).getTime();
  if (Number.isNaN(t)) return iso;
  const d = new Date(t + VN_OFFSET_MS);
  return `${pad(d.getUTCDate())}/${pad(d.getUTCMonth() + 1)}/${d.getUTCFullYear()} ${pad(d.getUTCHours())}:${pad(d.getUTCMinutes())}`;
}

export function describeDiscount(
  v: Pick<VoucherView, "discountType" | "discountValue" | "maxDiscount">,
): string {
  if (v.discountType === "percent") {
    const cap =
      v.maxDiscount > 0
        ? `, tối đa ${formatPrice(v.maxDiscount)}`
        : ", không giới hạn mức giảm";
    return `Giảm ${v.discountValue}%${cap}`;
  }
  if (v.discountType === "fixed") {
    return `Giảm ${formatPrice(v.discountValue)}`;
  }
  return "Không xác định";
}

export function describeMinSpend(minSpend: number): string {
  return minSpend > 0 ? formatPrice(minSpend) : "Không yêu cầu";
}

export function describeUsage(v: Pick<VoucherView, "used" | "quota">): string {
  return v.quota > 0 ? `${v.used} / ${v.quota}` : `${v.used} / không giới hạn`;
}

export function voucherState(
  v: Pick<VoucherView, "startsAt" | "endsAt" | "used" | "quota">,
  now: Date,
): VoucherState {
  const t = now.getTime();
  if (v.endsAt && new Date(v.endsAt).getTime() <= t) return "ended";
  if (v.quota > 0 && v.used >= v.quota) return "exhausted";
  if (v.startsAt && new Date(v.startsAt).getTime() > t) return "upcoming";
  return "active";
}

export function voucherHref(code: string): string {
  return `/admin/vouchers/${encodeURIComponent(code)}`;
}
