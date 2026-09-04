export interface Voucher {
  code: string;
  title: string;
  description: string;
  discountType: "fixed" | "percent" | "shipping";
  discountValue: number; // e.g. 50000 (fixed), 10 (10%), or 35000 (shipping)
  minOrderAmount: number;
  maxDiscount?: number;
  badge: string;
}

export const AVAILABLE_VOUCHERS: Voucher[] = [
  {
    code: "FREESHIP",
    title: "Miễn phí vận chuyển",
    description: "Giảm tối đa 35.000 VND phí ship cho mọi đơn hàng",
    discountType: "shipping",
    discountValue: 35000,
    minOrderAmount: 0,
    badge: "Freeship Xtra",
  },
  {
    code: "MARKET50K",
    title: "Giảm 50.000 VND",
    description: "Áp dụng cho đơn hàng từ 200.000 VND",
    discountType: "fixed",
    discountValue: 50000,
    minOrderAmount: 200000,
    badge: "Voucher Sàn",
  },
  {
    code: "SUPER10",
    title: "Giảm 10% đơn hàng",
    description: "Giảm 10% tối đa 50.000 VND cho đơn từ 100.000 VND",
    discountType: "percent",
    discountValue: 10,
    minOrderAmount: 100000,
    maxDiscount: 50000,
    badge: "Giảm 10%",
  },
  {
    code: "CHAOBANMOI",
    title: "Chào Bạn Mới 30K",
    description: "Giảm ngay 30.000 VND cho đơn hàng từ 50.000 VND",
    discountType: "fixed",
    discountValue: 30000,
    minOrderAmount: 50000,
    badge: "Bạn mới",
  },
];

export function calculateVoucherDiscount(
  voucher: Voucher | null,
  itemsSubtotal: number,
  shippingFee: number,
): {
  discountAmount: number;
  shippingDiscount: number;
  valid: boolean;
  reason?: string;
} {
  if (!voucher) {
    return { discountAmount: 0, shippingDiscount: 0, valid: true };
  }

  if (itemsSubtotal < voucher.minOrderAmount) {
    return {
      discountAmount: 0,
      shippingDiscount: 0,
      valid: false,
      reason: `Đơn hàng tối thiểu ${voucher.minOrderAmount.toLocaleString("vi-VN")} VND để dùng mã này.`,
    };
  }

  if (voucher.discountType === "shipping") {
    const shippingDiscount = Math.min(shippingFee, voucher.discountValue);
    return { discountAmount: 0, shippingDiscount, valid: true };
  }

  if (voucher.discountType === "fixed") {
    const discountAmount = Math.min(itemsSubtotal, voucher.discountValue);
    return { discountAmount, shippingDiscount: 0, valid: true };
  }

  if (voucher.discountType === "percent") {
    let discountAmount = Math.round(
      (itemsSubtotal * voucher.discountValue) / 100,
    );
    if (voucher.maxDiscount && discountAmount > voucher.maxDiscount) {
      discountAmount = voucher.maxDiscount;
    }
    return { discountAmount, shippingDiscount: 0, valid: true };
  }

  return { discountAmount: 0, shippingDiscount: 0, valid: true };
}
