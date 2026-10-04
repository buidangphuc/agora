import { PaymentMethod } from "@/generated/platform/payment/v1/payment_pb.js";

export interface PaymentOption {
  method: PaymentMethod;
  title: string;
  desc: string;
  /** Short text for the token icon slot (replaces the old emoji). */
  slot: string;
}

export const PAYMENT_OPTIONS: readonly PaymentOption[] = [
  {
    method: PaymentMethod.COD,
    title: "Thanh toán khi nhận hàng (COD)",
    desc: "Thanh toán bằng tiền mặt khi shipper giao hàng tận nơi.",
    slot: "COD",
  },
  {
    method: PaymentMethod.MOCK_MOMO,
    title: "Ví điện tử MoMo (Demo QR)",
    desc: "Mô phỏng quét mã QR MoMo để thanh toán tự động.",
    slot: "MoMo",
  },
  {
    method: PaymentMethod.MOCK_BANK,
    title: "Chuyển khoản Ngân hàng (Demo VietQR)",
    desc: "Mô phỏng chuyển khoản nhanh 24/7.",
    slot: "Bank",
  },
  {
    method: PaymentMethod.MOCK_CARD,
    title: "Thẻ Visa / Mastercard (Demo)",
    desc: "Mô phỏng thanh toán thẻ quốc tế an toàn.",
    slot: "Thẻ",
  },
];

export function paymentOptionFor(method: PaymentMethod): PaymentOption {
  return (
    PAYMENT_OPTIONS.find((o) => o.method === method) ??
    (PAYMENT_OPTIONS[0] as PaymentOption)
  );
}
