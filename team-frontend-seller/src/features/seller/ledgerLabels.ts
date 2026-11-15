const LEDGER_TYPE_LABELS: Record<string, string> = {
  ORDER_SETTLEMENT: "Thanh toán đơn hàng",
  PAYOUT: "Rút tiền",
  REFUND_DEDUCTION: "Hoàn tiền cho người mua",
  COMMISSION: "Phí hoa hồng",
  COMMISSION_REVERSAL: "Hoàn phí hoa hồng",
  SUBSCRIPTION_FEE: "Phí gói người bán",
  AD_SPEND: "Chi phí quảng cáo",
};

export function ledgerTypeLabel(type: string): string {
  if (!type) return "—";
  return Object.hasOwn(LEDGER_TYPE_LABELS, type)
    ? LEDGER_TYPE_LABELS[type]
    : type;
}
