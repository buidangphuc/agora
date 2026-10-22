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
      items: [],
      sv: {},
      pv: undefined,
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

  it("ignores an invalid pay", () => {
    expect(resolveCheckout({ pay: "99" }, addrs).pay).toBe(PaymentMethod.COD);
  });

  it("items are parsed from a comma-separated list; empty means whole cart", () => {
    expect(resolveCheckout({ items: "i1,i2" }, addrs).items).toEqual([
      "i1",
      "i2",
    ]);
    expect(resolveCheckout({ items: "" }, addrs).items).toEqual([]);
    expect(resolveCheckout({}, addrs).items).toEqual([]);
  });

  it("sv is repeatable sellerId:CODE with at most one code per seller; pv is one code", () => {
    const r = resolveCheckout(
      { sv: ["s1:SHOPA10", "s2:B5", "s1:LATER"], pv: "PLAT5" },
      addrs,
    );
    expect(r.sv).toEqual({ s1: "SHOPA10", s2: "B5" });
    expect(r.pv).toBe("PLAT5");
    expect(resolveCheckout({ sv: "s3:X" }, addrs).sv).toEqual({ s3: "X" });
  });

  it("a malformed sv entry is dropped", () => {
    expect(resolveCheckout({ sv: ["nocolon", ":X", "s1:"] }, addrs).sv).toEqual(
      {},
    );
  });

  it("legacy voucher param is ignored", () => {
    const r = resolveCheckout({ voucher: "OLD" }, addrs);
    expect(r).not.toHaveProperty("voucher");
    expect(r.sv).toEqual({});
    expect(r.pv).toBeUndefined();
    expect(r.needsRedirect).toBe(false);
    expect(checkoutHref({ ...r })).not.toContain("voucher");
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
  });

  it("items are kept by checkoutHref", () => {
    expect(checkoutHref({ items: ["i1", "i2"] })).toBe(
      "/checkout?items=i1%2Ci2",
    );
    expect(checkoutHref({ items: [] })).toBe("/checkout?");
  });

  it("items survive every checkout step", () => {
    for (const step of ["address", "shipping", "payment", "confirm"] as const) {
      const parsed = resolveCheckout(
        { step, addr: "a1", items: "i1,i2" },
        addrs,
      );
      expect(parsed.items).toEqual(["i1", "i2"]);
      expect(checkoutHref({ ...parsed })).toContain("items=i1%2Ci2");
    }
  });

  it("sv and pv round-trip through the href", () => {
    const href = checkoutHref({
      step: "confirm",
      sv: { s1: "SHOPA10", s2: "B5" },
      pv: "PLAT5",
    });
    const q = new URLSearchParams(href.split("?")[1]);
    expect(q.getAll("sv")).toEqual(["s1:SHOPA10", "s2:B5"]);
    expect(q.get("pv")).toBe("PLAT5");
    expect(q.has("voucher")).toBe(false);
    const back = resolveCheckout(
      {
        step: "confirm",
        addr: "a1",
        sv: q.getAll("sv"),
        pv: q.get("pv") ?? undefined,
      },
      addrs,
    );
    expect(back.sv).toEqual({ s1: "SHOPA10", s2: "B5" });
    expect(back.pv).toBe("PLAT5");
  });
});
