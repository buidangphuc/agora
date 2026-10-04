import { describe, expect, it } from "vitest";

import { orderTotal } from "./orderTotal";

describe("orderTotal", () => {
  it("applies discount and shipping", () => {
    expect(orderTotal(300000, 30000, 20000)).toBe(290000);
  });

  it("never goes below zero", () => {
    expect(orderTotal(10000, 50000, 0)).toBe(0);
  });

  it("is the subtotal when there is no discount and shipping is free", () => {
    expect(orderTotal(500000, 0, 0)).toBe(500000);
  });
});
