import { describe, expect, it } from "vitest";

import type { ViewCartItem } from "@/lib/gateway/cart";

import { groupByShop } from "./groupByShop";

function item(
  id: string,
  sellerId: string,
  unitPrice: number,
  quantity: number,
  sellerDisplayName = "",
): ViewCartItem {
  return {
    id,
    listingId: `l-${id}`,
    variantId: "",
    quantity,
    unitPrice,
    title: id,
    variantName: "",
    imageUrl: "",
    sellerId,
    sellerDisplayName,
  };
}

describe("groupByShop", () => {
  it("groups two sellers in first-seen order with per-shop subtotals", () => {
    const groups = groupByShop([
      item("a", "s1", 100, 2, "Alpha"),
      item("b", "s2", 50, 1, "Beta"),
      item("c", "s1", 10, 3, "Alpha"),
    ]);
    expect(groups.map((g) => g.sellerId)).toEqual(["s1", "s2"]);
    expect(groups[0]?.items.map((i) => i.id)).toEqual(["a", "c"]);
    expect(groups[0]?.subtotal).toBe(230);
    expect(groups[0]?.sellerDisplayName).toBe("Alpha");
    expect(groups[1]?.subtotal).toBe(50);
  });

  it("returns no groups for an empty cart", () => {
    expect(groupByShop([])).toEqual([]);
  });

  it("keeps an empty name empty so callers can fall back to the id label", () => {
    expect(groupByShop([item("a", "s1", 1, 1)])[0]?.sellerDisplayName).toBe("");
  });
});

describe("groupByShop combos", () => {
  const bundled = (id: string, bundleId: string, discount: number) => ({
    ...item(id, "s1", 100, 1),
    bundleId,
    bundleDiscount: discount,
  });

  it("C8 groups lines sharing a bundle id into one combo inside the shop group", () => {
    const groups = groupByShop([
      bundled("a", "b1", 10000),
      { ...item("r", "s1", 50, 1), bundleId: "", bundleDiscount: 0 },
      bundled("c", "b1", 20000),
    ]);
    expect(groups).toHaveLength(1);
    expect(groups[0]?.items.map((i) => i.id)).toEqual(["a", "r", "c"]);
    expect(groups[0]?.combos).toHaveLength(1);
    expect(groups[0]?.combos[0]?.bundleId).toBe("b1");
    expect(groups[0]?.combos[0]?.items.map((i) => i.id)).toEqual(["a", "c"]);
    expect(groups[0]?.subtotal).toBe(250);
  });

  it("C11 treats lines without bundle id as regular", () => {
    const groups = groupByShop([
      { ...item("a", "s1", 100, 1), bundleId: "", bundleDiscount: 5000 },
      { ...item("b", "s1", 100, 1), bundleId: "b1", bundleDiscount: 0 },
      item("c", "s1", 100, 1),
    ]);
    expect(groups[0]?.combos).toEqual([]);
    expect(groups[0]?.items.map((i) => i.id)).toEqual(["a", "b", "c"]);
  });
});
