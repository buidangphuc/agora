import { describe, expect, it } from "vitest";

import {
  EMPTY_VOUCHER_FORM,
  type VoucherFormValues,
  validateEditForm,
  validateVoucherForm,
} from "./voucherForm";
import {
  STATE_LABEL,
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
    active: true,
  };

  it("V28 derives the state from the response times and usage", () => {
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

  it("V28 a stopped voucher is stopped whatever its dates and usage", () => {
    const stopped = { ...base, active: false };
    expect(voucherState(stopped, NOW)).toBe("stopped");
    expect(voucherState(stopped, new Date("2026-11-15T00:00:00Z"))).toBe(
      "stopped",
    );
    expect(voucherState(stopped, new Date("2026-12-02T00:00:00Z"))).toBe(
      "stopped",
    );
    expect(
      voucherState({ ...stopped, used: 10 }, new Date("2026-11-15T00:00:00Z")),
    ).toBe("stopped");
    expect(STATE_LABEL.stopped).toBe("Đã ngừng");
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

describe("validateEditForm", () => {
  const current = {
    quota: 100,
    used: 5,
    startsAt: "2098-12-31T17:00:00.000Z",
    endsAt: "2099-01-31T17:00:00.000Z",
  };
  const ok = { endsAt: "2099-03-01T00:00", quota: "200" };

  it("V29 accepts a later end date and a higher quota, reading time as UTC+7", () => {
    const res = validateEditForm(ok, current, NOW);
    expect(res).toEqual({
      ok: true,
      input: { endsAt: new Date("2099-02-28T17:00:00.000Z"), quota: 200 },
    });
  });

  it("V29 sends only the fields that changed", () => {
    const sameQuota = validateEditForm({ ...ok, quota: "100" }, current, NOW);
    expect(sameQuota).toEqual({
      ok: true,
      input: { endsAt: new Date("2099-02-28T17:00:00.000Z") },
    });
    const sameEnd = validateEditForm(
      { endsAt: "2099-02-01T00:00", quota: "150" },
      current,
      NOW,
    );
    expect(sameEnd).toEqual({ ok: true, input: { quota: 150 } });
  });

  it.each([
    ["below the current quota", "50"],
    ["zero", "0"],
    ["empty", ""],
    ["not a number", "abc"],
    ["negative", "-5"],
    ["fractional", "1.5"],
  ])("V30 refuses a quota that is %s on the quota field", (_n, quota) => {
    const res = validateEditForm({ ...ok, quota }, current, NOW);
    expect(res.ok).toBe(false);
    if (!res.ok) {
      expect(res.errors.quota).toBeTruthy();
      expect(res.errors.endsAt).toBeUndefined();
    }
  });

  it("V30 refuses a quota below the used count", () => {
    const res = validateEditForm(
      { ...ok, quota: "7" },
      { ...current, quota: 5, used: 9 },
      NOW,
    );
    expect(res.ok).toBe(false);
    if (!res.ok) expect(res.errors.quota).toBeTruthy();
  });

  it.each([
    ["equal to the start", "2099-01-01T00:00"],
    ["before the start", "2098-12-01T00:00"],
    ["empty", ""],
    ["malformed", "tomorrow"],
  ])("V30 refuses an end date %s on the end date field", (_n, endsAt) => {
    const res = validateEditForm({ ...ok, endsAt }, current, NOW);
    expect(res.ok).toBe(false);
    if (!res.ok) {
      expect(res.errors.endsAt).toBeTruthy();
      expect(res.errors.quota).toBeUndefined();
    }
  });

  it("V30a ignores the quota of an unlimited voucher", () => {
    const res = validateEditForm(
      { endsAt: "2099-03-01T00:00", quota: "5" },
      { ...current, quota: 0 },
      NOW,
    );
    expect(res).toEqual({
      ok: true,
      input: { endsAt: new Date("2099-02-28T17:00:00.000Z") },
    });
  });
});
