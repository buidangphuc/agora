import { describe, expect, it } from "vitest";

import type { ViewVariant } from "@/lib/gateway/listings";
import {
  paginate,
  parsePdpParams,
  parseSort,
  pdpHref,
  resolveVariant,
  shouldWriteVariantToUrl,
  starCount,
} from "./pdp";

function variant(over: Partial<ViewVariant>): ViewVariant {
  return {
    id: "v",
    listingId: "l1",
    name: "V",
    sku: "",
    price: 0,
    stock: 5,
    imageUrl: "",
    ...over,
  };
}

const base = { price: 100, stock: 10 };

describe("resolveVariant", () => {
  it("returns the base listing when there are no variants", () => {
    const r = resolveVariant({ ...base, variants: [] }, "x");
    expect(r).toMatchObject({ variant: null, id: "", price: 100, stock: 10 });
  });

  it("uses the requested variant, with its own price, stock, sku and image", () => {
    const variants = [
      variant({ id: "a" }),
      variant({ id: "b", price: 250, stock: 3, sku: "S-B", imageUrl: "b.png" }),
    ];
    const r = resolveVariant({ ...base, variants }, "b");
    expect(r).toMatchObject({
      id: "b",
      price: 250,
      stock: 3,
      sku: "S-B",
      imageUrl: "b.png",
    });
  });

  it("falls back to the base price when the variant has no price", () => {
    const r = resolveVariant(
      { ...base, variants: [variant({ id: "a" })] },
      "a",
    );
    expect(r.price).toBe(100);
  });

  it("falls back to the first in-stock variant for an unknown id", () => {
    const variants = [variant({ id: "a", stock: 0 }), variant({ id: "b" })];
    expect(resolveVariant({ ...base, variants }, "nope").id).toBe("b");
  });

  it("picks the first in-stock variant when the first one is out of stock", () => {
    const variants = [variant({ id: "a", stock: 0 }), variant({ id: "b" })];
    expect(resolveVariant({ ...base, variants }, undefined).id).toBe("b");
  });

  it("falls back to the first variant when all are out of stock", () => {
    const variants = [
      variant({ id: "a", stock: 0 }),
      variant({ id: "b", stock: 0 }),
    ];
    expect(resolveVariant({ ...base, variants }, "nope").id).toBe("a");
  });

  it("keeps an explicitly requested out-of-stock variant", () => {
    const variants = [variant({ id: "a" }), variant({ id: "b", stock: 0 })];
    const r = resolveVariant({ ...base, variants }, "b");
    expect(r.id).toBe("b");
    expect(r.stock).toBe(0);
  });
});

describe("shouldWriteVariantToUrl", () => {
  const variants = [
    variant({ id: "same", price: 100, stock: 10 }),
    variant({ id: "noPrice", price: 0, stock: 10 }),
    variant({ id: "price", price: 200, stock: 10 }),
    variant({ id: "stock", price: 100, stock: 4 }),
  ];
  const listing = { ...base, variants };

  it("is false when price and stock equal the base", () => {
    expect(shouldWriteVariantToUrl(listing, "same")).toBe(false);
    expect(shouldWriteVariantToUrl(listing, "noPrice")).toBe(false);
  });

  it("is true when price or stock differs", () => {
    expect(shouldWriteVariantToUrl(listing, "price")).toBe(true);
    expect(shouldWriteVariantToUrl(listing, "stock")).toBe(true);
  });

  it("is false for an unknown variant", () => {
    expect(shouldWriteVariantToUrl(listing, "ghost")).toBe(false);
  });
});

describe("parsePdpParams", () => {
  it("applies defaults", () => {
    expect(parsePdpParams({})).toEqual({
      variant: undefined,
      rating: 0,
      rpage: 1,
      sort: "all",
    });
  });

  it("reads valid values and takes the first of repeated params", () => {
    expect(
      parsePdpParams({
        variant: ["v1", "v2"],
        rating: "4",
        rpage: "3",
        sort: "price_desc",
      }),
    ).toEqual({ variant: "v1", rating: 4, rpage: 3, sort: "price_desc" });
  });

  it("rejects an invalid rating", () => {
    for (const rating of ["0", "6", "-1", "abc", "2.5", ""]) {
      expect(parsePdpParams({ rating }).rating).toBe(0);
    }
  });

  it("rejects an invalid page and bounds a huge one", () => {
    for (const rpage of ["0", "-2", "x", "1.5", ""]) {
      expect(parsePdpParams({ rpage }).rpage).toBe(1);
    }
    expect(parsePdpParams({ rpage: "999999999" }).rpage).toBe(10000);
  });

  it("ignores a tab param entirely", () => {
    expect(Object.keys(parsePdpParams({ tab: "reviews" }))).not.toContain(
      "tab",
    );
  });
});

describe("parseSort", () => {
  it("accepts only the known sorts", () => {
    expect(parseSort("price_asc")).toBe("price_asc");
    expect(parseSort("price_desc")).toBe("price_desc");
    expect(parseSort("name")).toBe("all");
    expect(parseSort(undefined)).toBe("all");
  });
});

describe("paginate", () => {
  const items = Array.from({ length: 23 }, (_, i) => i + 1);

  it("slices 10 per page", () => {
    expect(paginate(items, 1, 10).items).toHaveLength(10);
    const third = paginate(items, 3, 10);
    expect(third.items).toEqual([21, 22, 23]);
    expect(third).toMatchObject({ page: 3, pages: 3, total: 23 });
  });

  it("clamps an out-of-range page", () => {
    expect(paginate(items, 99, 10).page).toBe(3);
    expect(paginate(items, 0, 10).page).toBe(1);
  });

  it("handles an empty list", () => {
    expect(paginate([], 1, 10)).toEqual({
      items: [],
      page: 1,
      pages: 1,
      total: 0,
    });
  });
});

describe("pdpHref", () => {
  it("omits defaults", () => {
    expect(pdpHref("l1", {})).toBe("/listing/l1");
    expect(pdpHref("l1", { rating: 0, rpage: 1 }, "reviews")).toBe(
      "/listing/l1#reviews",
    );
  });

  it("keeps variant, rating and page with a hash", () => {
    expect(
      pdpHref("l1", { variant: "v2", rating: 4, rpage: 2 }, "reviews"),
    ).toBe("/listing/l1?variant=v2&rating=4&rpage=2#reviews");
  });
});

describe("starCount", () => {
  it("reads the breakdown by star", () => {
    const b = { star1: 1, star2: 2, star3: 3, star4: 4, star5: 5 };
    expect([5, 4, 3, 2, 1].map((s) => starCount(b, s))).toEqual([
      5, 4, 3, 2, 1,
    ]);
  });
});
