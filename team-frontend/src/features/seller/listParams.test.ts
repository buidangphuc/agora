import { describe, expect, it } from "vitest";

import { buildListHref, parseListParams, slicePage } from "./listParams";

const STATUSES = ["published", "draft"] as const;

describe("parseListParams", () => {
  it("defaults an empty query string", () => {
    expect(parseListParams({}, STATUSES)).toEqual({
      q: "",
      status: "all",
      page: 1,
    });
  });

  it("reads valid values", () => {
    expect(
      parseListParams({ q: " iphone ", status: "draft", page: "3" }, STATUSES),
    ).toEqual({ q: "iphone", status: "draft", page: 3 });
  });

  it("falls back for invalid values without throwing", () => {
    for (const page of ["abc", "-2", "0", "1.5", "", "99999999999999999999"]) {
      const { page: p } = parseListParams({ page }, STATUSES);
      expect(Number.isInteger(p)).toBe(true);
      expect(p).toBeGreaterThanOrEqual(1);
    }
    expect(parseListParams({ page: "abc" }, STATUSES).page).toBe(1);
    expect(parseListParams({ status: "bogus" }, STATUSES).status).toBe("all");
  });

  it("takes the first of repeated params and caps the query length", () => {
    expect(parseListParams({ q: ["a", "b"] }, STATUSES).q).toBe("a");
    expect(parseListParams({ q: "x".repeat(500) }, STATUSES).q).toHaveLength(
      100,
    );
  });
});

describe("buildListHref", () => {
  it("omits defaults and encodes values", () => {
    expect(buildListHref("/seller", { q: "", status: "all", page: 1 })).toBe(
      "/seller",
    );
    expect(
      buildListHref("/seller", { q: "áo thun", status: "draft", page: 2 }),
    ).toBe("/seller?q=%C3%A1o+thun&status=draft&page=2");
  });
});

describe("slicePage", () => {
  it("slices 1-based pages", () => {
    const items = Array.from({ length: 45 }, (_, i) => i + 1);
    expect(slicePage(items, 2, 20)[0]).toBe(21);
    expect(slicePage(items, 3, 20)).toHaveLength(5);
    expect(slicePage(items, 9, 20)).toEqual([]);
  });
});
