import { describe, expect, it } from "vitest";

import {
  activeFilterCount,
  buildSearchHref,
  clearFiltersHref,
  parseSearchParams,
  toggleAttr,
} from "./url";

describe("parseSearchParams", () => {
  it("defaults everything for an empty query string", () => {
    expect(parseSearchParams({})).toEqual({
      q: "",
      category: "",
      seller: "",
      minPrice: undefined,
      maxPrice: undefined,
      sort: "relevance",
      attrs: {},
      page: 1,
    });
  });

  it("reads all params", () => {
    expect(
      parseSearchParams({
        q: " ao ",
        category: "c1",
        seller: "s1",
        minPrice: "100000",
        maxPrice: "500000",
        sort: "price_asc",
        page: "3",
      }),
    ).toEqual({
      q: "ao",
      category: "c1",
      seller: "s1",
      minPrice: 100000,
      maxPrice: 500000,
      sort: "price_asc",
      attrs: {},
      page: 3,
    });
  });

  it("falls back to defaults for unknown or malformed values", () => {
    const s = parseSearchParams({
      sort: "sales",
      minPrice: "abc",
      maxPrice: "-5",
      page: "0",
    });
    expect(s.sort).toBe("relevance");
    expect(s.minPrice).toBeUndefined();
    expect(s.maxPrice).toBeUndefined();
    expect(s.page).toBe(1);
    expect(parseSearchParams({ page: "x" }).page).toBe(1);
    expect(parseSearchParams({ page: ["2", "3"] }).page).toBe(2);
  });
});

describe("buildSearchHref", () => {
  const base = parseSearchParams({ q: "ao", sort: "newest" });

  it("omits defaults", () => {
    expect(buildSearchHref(parseSearchParams({}))).toBe("/search");
    expect(buildSearchHref(base)).toBe("/search?q=ao&sort=newest");
  });

  it("keeps the other params when building page links", () => {
    expect(buildSearchHref(base, { page: 2 })).toBe(
      "/search?q=ao&sort=newest&page=2",
    );
    expect(buildSearchHref(base, { page: 1 })).toBe("/search?q=ao&sort=newest");
  });

  it("encodes a price bucket and sort in the shareable URL", () => {
    expect(
      buildSearchHref(parseSearchParams({}), {
        minPrice: 100000,
        maxPrice: 500000,
        sort: "price_asc",
      }),
    ).toBe("/search?minPrice=100000&maxPrice=500000&sort=price_asc");
  });

  it("resets the page when a filter or sort changes", () => {
    const onPage3 = parseSearchParams({ q: "ao", page: "3" });
    expect(buildSearchHref(onPage3, { category: "c1" })).toBe(
      "/search?q=ao&category=c1",
    );
    expect(buildSearchHref(onPage3, { sort: "newest" })).toBe(
      "/search?q=ao&sort=newest",
    );
  });

  it("removes one filter at a time", () => {
    const s = parseSearchParams({ q: "ao", category: "c1", seller: "s1" });
    expect(buildSearchHref(s, { category: "" })).toBe("/search?q=ao&seller=s1");
  });

  it("encodes special characters in the keyword", () => {
    expect(buildSearchHref(parseSearchParams({ q: "áo khoác" }))).toBe(
      "/search?q=%C3%A1o+kho%C3%A1c",
    );
  });
});

describe("an old rating link", () => {
  it("parses equal to the same URL without rating", () => {
    expect(parseSearchParams({ q: "x", rating: "4" })).toEqual(
      parseSearchParams({ q: "x" }),
    );
    expect(parseSearchParams({ q: "x", rating: "4" })).not.toHaveProperty(
      "rating",
    );
  });

  it("never reaches a built href and is not an active filter", () => {
    const s = parseSearchParams({ q: "x", rating: "4", sort: "newest" });
    expect(buildSearchHref(s)).toBe("/search?q=x&sort=newest");
    expect(buildSearchHref(s, { page: 2 })).not.toContain("rating=");
    expect(clearFiltersHref(s)).not.toContain("rating=");
    expect(activeFilterCount(s)).toBe(0);
  });
});

describe("filters", () => {
  it("counts filter groups, with price counted once", () => {
    expect(activeFilterCount(parseSearchParams({ q: "ao" }))).toBe(0);
    expect(
      activeFilterCount(
        parseSearchParams({ category: "c1", minPrice: "1", maxPrice: "9" }),
      ),
    ).toBe(2);
  });

  it("clears every filter but keeps the keyword", () => {
    const s = parseSearchParams({
      q: "ao",
      category: "c1",
      minPrice: "1",
      sort: "newest",
    });
    expect(clearFiltersHref(s)).toBe("/search?q=ao&sort=newest");
    expect(clearFiltersHref(parseSearchParams({ category: "c1" }))).toBe(
      "/search",
    );
  });
});

describe("dynamic facet params (tag.<group> / sku.<group>)", () => {
  it("parses comma lists, drops malformed groups and slugs, ignores unknown prefixes", () => {
    const s = parseSearchParams({
      "sku.color": "xanh-navy, den,den",
      "tag.connectivity": "bluetooth-5-3",
      "tag.Bad": "x",
      "sku.size": "Xl L",
      "foo.color": "x",
      "sku.ram": "",
    });
    expect(s.attrs).toEqual({
      "sku.color": ["xanh-navy", "den"],
      "tag.connectivity": ["bluetooth-5-3"],
    });
  });

  it("round-trips through buildSearchHref in stable key order and resets the page", () => {
    const s = parseSearchParams({
      q: "ao",
      page: "3",
      "tag.feature": "chong-nuoc",
      "sku.color": "den,trang",
    });
    expect(buildSearchHref(s, { page: 3 })).toBe(
      "/search?q=ao&sku.color=den%2Ctrang&tag.feature=chong-nuoc&page=3",
    );
    expect(buildSearchHref(s)).not.toContain("page=");
    const again = parseSearchParams(
      Object.fromEntries(
        new URL(`http://x${buildSearchHref(s)}`).searchParams.entries(),
      ),
    );
    expect(again.attrs).toEqual(s.attrs);
  });

  it("toggles one slug, drops an emptied group, counts groups and clears with the filters", () => {
    const s = parseSearchParams({ q: "ao", "sku.color": "den" });
    expect(toggleAttr(s.attrs, "sku.color", "trang")).toEqual({
      "sku.color": ["den", "trang"],
    });
    expect(toggleAttr(s.attrs, "sku.color", "den")).toEqual({});
    expect(activeFilterCount(s)).toBe(1);
    expect(clearFiltersHref(s)).toBe("/search?q=ao");
  });
});
