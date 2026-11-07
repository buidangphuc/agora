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

describe("parseSearchParams sort and minRating (frontend-discovery-sort-rating)", () => {
  it("Best-selling sort: parses sort=best_selling", () => {
    expect(parseSearchParams({ sort: "best_selling" }).sort).toBe(
      "best_selling",
    );
  });

  it("An unknown sort value falls back to relevance", () => {
    expect(parseSearchParams({ sort: "top_rated" }).sort).toBe("relevance");
    expect(parseSearchParams({ sort: "BEST_SELLING" }).sort).toBe("relevance");
  });

  it("The search page offers a rating filter: minRating 4 and 3 are accepted", () => {
    expect(parseSearchParams({ minRating: "4" }).minRating).toBe(4);
    expect(parseSearchParams({ minRating: "3" }).minRating).toBe(3);
  });

  it("An unsupported minRating value is ignored", () => {
    for (const v of [
      "9",
      "-1",
      "0",
      "1",
      "2",
      "5",
      "abc",
      "",
      "4.5",
      "04",
      "3 ",
    ]) {
      const s = parseSearchParams({ q: "x", minRating: v });
      expect(s.minRating).toBeUndefined();
    }
    expect(parseSearchParams({ q: "x", minRating: "9" })).toEqual(
      parseSearchParams({ q: "x" }),
    );
  });

  it("A repeated minRating uses the first value", () => {
    expect(parseSearchParams({ minRating: ["4", "3"] }).minRating).toBe(4);
    expect(parseSearchParams({ minRating: ["3", "4"] }).minRating).toBe(3);
    expect(
      parseSearchParams({ minRating: ["9", "4"] }).minRating,
    ).toBeUndefined();
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

describe("buildSearchHref sort and minRating (frontend-discovery-sort-rating)", () => {
  const q = (href: string) => new URL(`http://x${href}`).searchParams;

  it("Best-selling sort: builds sort=best_selling and round-trips", () => {
    const href = buildSearchHref(parseSearchParams({ q: "ao" }), {
      sort: "best_selling",
    });
    expect(href).toBe("/search?q=ao&sort=best_selling");
    expect(parseSearchParams(Object.fromEntries(q(href))).sort).toBe(
      "best_selling",
    );
  });

  it("Choosing a rating filters the results by URL: builds minRating and keeps the other params", () => {
    const href = buildSearchHref(
      parseSearchParams({ q: "ao", sort: "newest", category: "c1" }),
      { minRating: 4 },
    );
    const p = q(href);
    expect(p.get("minRating")).toBe("4");
    expect(p.get("q")).toBe("ao");
    expect(p.get("sort")).toBe("newest");
    expect(p.get("category")).toBe("c1");
  });

  it("keeps minRating in sort and page links", () => {
    const s = parseSearchParams({ q: "ao", minRating: "3" });
    expect(q(buildSearchHref(s, { sort: "newest" })).get("minRating")).toBe(
      "3",
    );
    expect(q(buildSearchHref(s, { page: 2 })).get("minRating")).toBe("3");
  });

  it("Changing a filter resets the page when the rating changes", () => {
    const onPage3 = parseSearchParams({ q: "ao", page: "3" });
    expect(buildSearchHref(onPage3, { minRating: 4 })).toBe(
      "/search?q=ao&minRating=4",
    );
    const rated = parseSearchParams({ q: "ao", page: "3", minRating: "4" });
    expect(buildSearchHref(rated, { minRating: 3 })).not.toContain("page=");
  });

  it("The rating chip removes the filter: minRating undefined drops the param", () => {
    const s = parseSearchParams({ q: "ao", minRating: "4" });
    expect(buildSearchHref(s, { minRating: undefined })).toBe("/search?q=ao");
  });

  it("never writes an out-of-range minRating", () => {
    const s = parseSearchParams({ q: "ao", minRating: "9" });
    expect(buildSearchHref(s)).toBe("/search?q=ao");
  });

  it("counts the rating once and clear-all drops it", () => {
    const s = parseSearchParams({ q: "ao", minRating: "4" });
    expect(activeFilterCount(s)).toBe(1);
    expect(
      activeFilterCount(parseSearchParams({ category: "c1", minRating: "3" })),
    ).toBe(2);
    expect(clearFiltersHref(s)).toBe("/search?q=ao");
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

  it("An old rating link renders the results without error: rating is not minRating", () => {
    for (const r of ["4", "3", "5"]) {
      const s = parseSearchParams({ q: "x", rating: r });
      expect(s.minRating).toBeUndefined();
      expect(buildSearchHref(s)).toBe("/search?q=x");
    }
    const both = parseSearchParams({ q: "x", rating: "4", minRating: "3" });
    expect(both.minRating).toBe(3);
    expect(buildSearchHref(both)).not.toContain("rating=4");
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
