import { describe, expect, it } from "vitest";

import {
  EMPTY_VOUCHER_FORM,
  type VoucherFormValues,
  validateVoucherForm,
} from "./voucherForm";
import {
  describeDiscount,
  describeUsage,
  formatVnDateTime,
  voucherHref,
  voucherState,
} from "./voucherFormat";

const NOW = new Date("2026-10-30T03:00:00.000Z");

const valid: VoucherFormValues = {
  code: " sale11 ",
  discountType: "percent",
  discountValue: "10",
  minSpend: "100000",
  maxDiscount: "50000",
  quota: "1000",
  startsAt: "2026-11-01T00:00",
  endsAt: "2026-12-01T00:00",
};

describe("validateVoucherForm", () => {
  it("accepts a valid percent voucher, upper-cases the code and reads times as UTC+7", () => {
    const res = validateVoucherForm(valid, NOW);
    expect(res).toEqual({
      ok: true,
      input: {
        code: "SALE11",
        discountType: "percent",
        discountValue: 10,
        minSpend: 100000,
        maxDiscount: 50000,
        quota: 1000,
        startsAt: new Date("2026-10-31T17:00:00.000Z"),
        endsAt: new Date("2026-11-30T17:00:00.000Z"),
      },
    });
  });

  it("drops the cap for a fixed voucher and treats blank optional amounts as 0", () => {
    const res = validateVoucherForm(
      {
        ...valid,
        discountType: "fixed",
        discountValue: "20000",
        maxDiscount: "abc",
        minSpend: "",
        quota: "",
      },
      NOW,
    );
    expect(res.ok).toBe(true);
    if (res.ok) {
      expect(res.input.maxDiscount).toBe(0);
      expect(res.input.minSpend).toBe(0);
      expect(res.input.quota).toBe(0);
    }
  });

  it("reports every invalid field of the empty form", () => {
    const res = validateVoucherForm(EMPTY_VOUCHER_FORM, NOW);
    expect(res.ok).toBe(false);
    if (res.ok) return;
    expect(Object.keys(res.errors).sort()).toEqual([
      "code",
      "discountValue",
      "endsAt",
      "startsAt",
    ]);
  });

  it.each([
    ["ab", "code"],
    ["CÓDẤU", "code"],
    ["A".repeat(33), "code"],
  ])("rejects code %s", (code, field) => {
    const res = validateVoucherForm({ ...valid, code }, NOW);
    expect(res.ok).toBe(false);
    if (!res.ok) expect(res.errors).toHaveProperty(field);
  });

  it.each(["0", "101", "1.5", "-3"])("rejects percent %s", (discountValue) => {
    const res = validateVoucherForm({ ...valid, discountValue }, NOW);
    expect(res.ok).toBe(false);
    if (!res.ok) expect(res.errors.discountValue).toMatch(/1 đến 100/);
  });

  it("rejects a zero fixed amount and an unknown type", () => {
    const fixed = validateVoucherForm(
      { ...valid, discountType: "fixed", discountValue: "0" },
      NOW,
    );
    expect(fixed.ok).toBe(false);
    const other = validateVoucherForm({ ...valid, discountType: "bogo" }, NOW);
    expect(other.ok).toBe(false);
    if (!other.ok) expect(other.errors.discountType).toBeTruthy();
  });

  it("rejects negative or non-integer amounts", () => {
    const res = validateVoucherForm(
      { ...valid, minSpend: "-1", maxDiscount: "1e3", quota: "2.5" },
      NOW,
    );
    expect(res.ok).toBe(false);
    if (res.ok) return;
    expect(Object.keys(res.errors).sort()).toEqual([
      "maxDiscount",
      "minSpend",
      "quota",
    ]);
  });

  it("requires the end after the start and in the future", () => {
    const before = validateVoucherForm(
      { ...valid, endsAt: "2026-11-01T00:00" },
      NOW,
    );
    expect(before.ok).toBe(false);
    if (!before.ok) expect(before.errors.endsAt).toMatch(/sau thời điểm/);
    const past = validateVoucherForm(
      { ...valid, startsAt: "2026-10-01T00:00", endsAt: "2026-10-30T09:00" },
      NOW,
    );
    expect(past.ok).toBe(false);
    if (!past.ok) expect(past.errors.endsAt).toMatch(/tương lai/);
  });
});

describe("voucher formatting", () => {
  const base = {
    startsAt: "2026-11-01T00:00:00.000Z",
    endsAt: "2026-12-01T00:00:00.000Z",
    used: 0,
    quota: 10,
  };

  it("derives the state from the response times and usage", () => {
    expect(voucherState(base, NOW)).toBe("upcoming");
    expect(voucherState(base, new Date("2026-11-15T00:00:00Z"))).toBe("active");
    expect(voucherState(base, new Date("2026-12-02T00:00:00Z"))).toBe("ended");
    expect(
      voucherState({ ...base, used: 10 }, new Date("2026-11-15T00:00:00Z")),
    ).toBe("exhausted");
    expect(
      voucherState(
        { ...base, used: 10, quota: 0 },
        new Date("2026-11-15T00:00:00Z"),
      ),
    ).toBe("active");
  });

  it("describes discounts, usage and Vietnam time", () => {
    expect(
      describeDiscount({
        discountType: "percent",
        discountValue: 10,
        maxDiscount: 0,
      }),
    ).toBe("Giảm 10%, không giới hạn mức giảm");
    expect(
      describeDiscount({
        discountType: "fixed",
        discountValue: 20000,
        maxDiscount: 0,
      }),
    ).toMatch(/^Giảm ₫20\.000$/);
    expect(describeUsage({ used: 3, quota: 0 })).toBe("3 / không giới hạn");
    expect(formatVnDateTime("2026-10-31T17:00:00.000Z")).toBe(
      "01/11/2026 00:00",
    );
    expect(formatVnDateTime("")).toBe("Không đặt");
    expect(voucherHref("A B/C")).toBe("/admin/vouchers/A%20B%2FC");
  });
});
