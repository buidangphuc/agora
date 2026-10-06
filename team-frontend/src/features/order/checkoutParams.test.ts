import { describe, expect, it } from "vitest";

import { PaymentMethod } from "@/generated/platform/payment/v1/payment_pb.js";

import { checkoutHref, resolveCheckout } from "./checkoutParams";

const addrs = [{ id: "a1" }, { id: "a2", isDefault: true }];

describe("resolveCheckout", () => {
  it("defaults to the address step with the default address and COD", () => {
    expect(resolveCheckout({}, addrs)).toEqual({
      step: "address",
      addr: "a2",
      pay: PaymentMethod.COD,
      voucher: undefined,
      needsRedirect: false,
    });
  });

  it("falls back to the first address when none is default", () => {
    expect(resolveCheckout({}, [{ id: "x" }, { id: "y" }]).addr).toBe("x");
  });

  it("treats an unknown step as address", () => {
    expect(resolveCheckout({ step: "bogus" }, addrs).step).toBe("address");
  });

  it("accepts every later step when addr is valid", () => {
    for (const step of ["shipping", "payment", "confirm"] as const) {
      const r = resolveCheckout({ step, addr: "a1", pay: "3" }, addrs);
      expect(r).toMatchObject({
        step,
        addr: "a1",
        pay: PaymentMethod.MOCK_BANK,
        needsRedirect: false,
      });
    }
  });

  it("redirects a skip-ahead with no address to the address step", () => {
    const r = resolveCheckout({ step: "confirm" }, addrs);
    expect(r.step).toBe("address");
    expect(r.needsRedirect).toBe(true);
  });

  it("redirects when addr is stale (deleted) and falls back to the default", () => {
    const r = resolveCheckout({ step: "payment", addr: "gone" }, addrs);
    expect(r).toMatchObject({
      step: "address",
      addr: "a2",
      needsRedirect: true,
    });
  });

  it("replaces a stale addr on the address step without redirecting", () => {
    const r = resolveCheckout({ step: "address", addr: "gone" }, addrs);
    expect(r).toMatchObject({ step: "address", addr: "a2" });
    expect(r.needsRedirect).toBe(false);
  });

  it("with no addresses every later step redirects and addr is undefined", () => {
    const r = resolveCheckout({ step: "shipping", addr: "a1" }, []);
    expect(r).toMatchObject({
      step: "address",
      addr: undefined,
      needsRedirect: true,
    });
  });

  it("ignores an invalid pay and keeps the voucher code", () => {
    const r = resolveCheckout({ pay: "99", voucher: "SAVE10" }, addrs);
    expect(r.pay).toBe(PaymentMethod.COD);
    expect(r.voucher).toBe("SAVE10");
  });

  it("reads the first value of a repeated param", () => {
    expect(resolveCheckout({ step: ["address", "confirm"] }, addrs).step).toBe(
      "address",
    );
  });
});

describe("checkoutHref", () => {
  it("serialises only the set fields", () => {
    expect(
      checkoutHref({ step: "payment", addr: "a1", pay: PaymentMethod.COD }),
    ).toBe("/checkout?step=payment&addr=a1&pay=1");
    expect(checkoutHref({ voucher: "SAVE10" })).toBe(
      "/checkout?voucher=SAVE10",
    );
  });
});
