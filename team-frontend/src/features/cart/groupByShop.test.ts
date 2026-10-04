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
