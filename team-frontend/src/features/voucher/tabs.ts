import { DiscountType, type ViewVoucher } from "@/lib/gateway/promotion";

export type VoucherTab = "all" | "shipping" | "fixed" | "percent";

export const VOUCHER_TABS: { id: VoucherTab; label: string }[] = [
  { id: "all", label: "Tất cả" },
  { id: "shipping", label: "Freeship" },
  { id: "fixed", label: "Giảm tiền" },
  { id: "percent", label: "Giảm %" },
];

export function parseVoucherTab(
  raw: string | string[] | undefined,
): VoucherTab {
  const v = Array.isArray(raw) ? raw[0] : raw;
  return VOUCHER_TABS.some((t) => t.id === v) ? (v as VoucherTab) : "all";
}

/**
 * Whether a voucher belongs to a tab. The promotion service only has PERCENT
 * and FIXED discounts, so "shipping" matches nothing until a shipping voucher
 * type exists (the tab then shows its empty state, nothing is faked).
 */
export function inTab(v: ViewVoucher, tab: VoucherTab): boolean {
  switch (tab) {
    case "all":
      return true;
    case "fixed":
      return v.discountType === DiscountType.FIXED;
    case "percent":
      return v.discountType === DiscountType.PERCENT;
    default:
      return false;
  }
}

export function tabCounts(vouchers: ViewVoucher[]): Record<VoucherTab, number> {
  return {
    all: vouchers.length,
    shipping: vouchers.filter((v) => inTab(v, "shipping")).length,
    fixed: vouchers.filter((v) => inTab(v, "fixed")).length,
    percent: vouchers.filter((v) => inTab(v, "percent")).length,
  };
}

export function voucherTabHref(tab: VoucherTab): string {
  return tab === "all" ? "/vouchers" : `/vouchers?type=${tab}`;
}
