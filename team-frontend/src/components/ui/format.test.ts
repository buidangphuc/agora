import { describe, expect, it } from "vitest";

import { formatXu } from "./format";

describe("formatXu", () => {
  it("S17 groups thousands with vi-VN dots and the xu suffix", () => {
    expect(formatXu(10000)).toBe("10.000 xu");
    expect(formatXu(20000)).toBe("20.000 xu");
    expect(formatXu(1234567)).toBe("1.234.567 xu");
  });

  it("S19 renders zero as 0 xu", () => {
    expect(formatXu(0)).toBe("0 xu");
  });

  it("S17 renders small values without grouping", () => {
    expect(formatXu(999)).toBe("999 xu");
  });

  it("S17 drops the fraction and never uses a currency symbol", () => {
    expect(formatXu(1500.7)).toMatch(/^1\.50[01] xu$/);
    expect(formatXu(10000)).not.toMatch(/[đ₫]/);
  });
});
