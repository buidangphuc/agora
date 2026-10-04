import { describe, expect, it } from "vitest";

import { computeShippingFee } from "./shipping";

describe("computeShippingFee", () => {
  it("is free from 500000 regardless of city", () => {
    expect(computeShippingFee("Đà Nẵng", 500000)).toEqual({
      fee: 0,
      isFree: true,
    });
    expect(computeShippingFee("", 1200000)).toEqual({ fee: 0, isFree: true });
  });

  it("is 20000 for Ho Chi Minh and Ha Noi below the threshold", () => {
    for (const city of [
      "Hồ Chí Minh",
      "TP. HCM",
      "Hà Nội",
      "hn",
      "ha noi hn",
    ]) {
      expect(computeShippingFee(city, 499999)).toEqual({
        fee: 20000,
        isFree: false,
      });
    }
  });

  it("is 35000 for any other city below the threshold", () => {
    expect(computeShippingFee("Đà Nẵng", 100000)).toEqual({
      fee: 35000,
      isFree: false,
    });
    expect(computeShippingFee("", 0)).toEqual({ fee: 35000, isFree: false });
  });
});
