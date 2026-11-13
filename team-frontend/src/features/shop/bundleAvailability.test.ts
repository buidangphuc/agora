import { describe, expect, it } from "vitest";

import { bundleAvailability } from "./bundleAvailability";

const bundle = (over: Partial<Parameters<typeof bundleAvailability>[0]> = {}) => ({
  id: "b1",
  sellerId: "s1",
  title: "Combo",
  listingIds: ["l1", "l2"],
  bundlePrice: 120000,
  createdAt: "",
  ...over,
});

const listing = (id: string, price: number) => ({ id, price, title: `T-${id}` });

describe("bundleAvailability", () => {
  it("[C7] is available with the saving when all listings are published and priced", () => {
    const r = bundleAvailability(bundle(), [listing("l1", 100000), listing("l2", 50000)]);
    expect(r).toEqual({ available: true, saving: 30000 });
  });

  it("[C2] is unavailable when a listing is missing from the shop listings", () => {
    const r = bundleAvailability(bundle(), [listing("l1", 100000)]);
    expect(r.available).toBe(false);
  });

  it("[C2] is unavailable when the shop has no listings", () => {
    expect(bundleAvailability(bundle(), []).available).toBe(false);
  });

  it("[C7] shows no saving when a listing price is unknown (0)", () => {
    const r = bundleAvailability(bundle(), [listing("l1", 100000), listing("l2", 0)]);
    expect(r).toEqual({ available: true, saving: null });
  });

  it("[C7] shows no saving when the bundle is not cheaper than the regular sum", () => {
    const r = bundleAvailability(bundle({ bundlePrice: 150000 }), [
      listing("l1", 100000),
      listing("l2", 50000),
    ]);
    expect(r.available).toBe(true);
    expect(r.saving).toBeNull();
  });

  it("[C2] ignores extra shop listings outside the bundle", () => {
    const r = bundleAvailability(bundle(), [
      listing("l1", 100000),
      listing("l2", 50000),
      listing("l3", 1),
    ]);
    expect(r).toEqual({ available: true, saving: 30000 });
  });
});
