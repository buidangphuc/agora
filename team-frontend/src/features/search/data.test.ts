import { beforeEach, describe, expect, it, vi } from "vitest";

const searchListings = vi.hoisted(() => vi.fn());

vi.mock("@/lib/gateway/listings", () => ({ listCategories: async () => [] }));
vi.mock("@/lib/gateway/search", () => ({
  SortBy: {
    UNSPECIFIED: 0,
    RELEVANCE: 1,
    NEWEST: 2,
    PRICE_ASC: 3,
    PRICE_DESC: 4,
    BEST_SELLING: 5,
  },
  searchListings,
}));

import { loadSearch, searchKey } from "./data";
import { parseSearchParams } from "./url";

const RESULT = {
  items: [],
  total: 0,
  page: 1,
  pageSize: 24,
  facets: {},
};

beforeEach(() => {
  searchListings.mockReset();
  searchListings.mockResolvedValue(RESULT);
});

async function optsFor(raw: Record<string, string | string[]>) {
  await loadSearch(searchKey(parseSearchParams(raw)));
  return searchListings.mock.calls.at(-1)?.[1] as Record<string, unknown>;
}

describe("loadSearch request mapping", () => {
  it("Best-selling sort: sort=best_selling maps to SORT_BY_BEST_SELLING", async () => {
    expect((await optsFor({ q: "ao", sort: "best_selling" })).sortBy).toBe(5);
  });

  it("An unknown sort value falls back to relevance", async () => {
    expect((await optsFor({ q: "ao", sort: "top_rated" })).sortBy).toBe(1);
  });

  it("Choosing a rating filters the results by URL: minRating reaches the gateway request", async () => {
    expect((await optsFor({ q: "ao", minRating: "4" })).minRating).toBe(4);
    expect(
      (await optsFor({ q: "ao", minRating: "3&x" })).minRating,
    ).toBeUndefined();
    expect((await optsFor({ q: "ao", minRating: "3" })).minRating).toBe(3);
  });

  it("An unsupported minRating value is ignored: no minimum rating is requested", async () => {
    for (const v of ["9", "-1", "abc"]) {
      const o = await optsFor({ q: `ao${v}`, minRating: v });
      expect(o.minRating ?? 0).toBe(0);
    }
  });

  it("An old rating link renders the results without error: rating= requests no minimum rating", async () => {
    const o = await optsFor({ q: "old", rating: "4" });
    expect(o.minRating ?? 0).toBe(0);
  });
});
