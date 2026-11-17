import { describe, expect, it } from "vitest";

import { ledgerTypeLabel } from "./ledgerLabels";

const KNOWN: Record<string, string> = {
  ORDER_SETTLEMENT: "Thanh toán đơn hàng",
  PAYOUT: "Rút tiền",
  REFUND_DEDUCTION: "Hoàn tiền cho người mua",
  COMMISSION: "Phí hoa hồng",
  COMMISSION_REVERSAL: "Hoàn phí hoa hồng",
  SUBSCRIPTION_FEE: "Phí gói người bán",
  AD_SPEND: "Chi phí quảng cáo",
};

describe("Every known type has a label", () => {
  it("labels every known type", () => {
    for (const [type, label] of Object.entries(KNOWN)) {
      expect(ledgerTypeLabel(type)).toBe(label);
      expect(ledgerTypeLabel(type)).not.toBe(type);
    }
  });
});

describe("Unknown or empty type falls back", () => {
  it("falls back for unknown and empty types", () => {
    expect(ledgerTypeLabel("FUTURE_TYPE")).toBe("FUTURE_TYPE");
    expect(ledgerTypeLabel("")).toBe("—");
    expect(ledgerTypeLabel("commission")).toBe("commission");
  });
});
