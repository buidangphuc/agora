import { describe, expect, it } from "vitest";

import {
  buildListHref,
  parseListParams,
  parseMarketingParams,
  slicePage,
} from "./listParams";

const STATUSES = ["published", "draft"] as const;

describe("parseListParams", () => {
  it("defaults an empty query string", () => {
    expect(parseListParams({}, STATUSES)).toEqual({
      q: "",
      status: "all",
      page: 1,
      cursor: "",
    });
  });

  it("reads valid values", () => {
    expect(
      parseListParams({ q: " iphone ", status: "draft", page: "3" }, STATUSES),
    ).toEqual({ q: "iphone", status: "draft", page: 3, cursor: "" });
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

describe("cursor paging params", () => {
  it("Next page uses the returned cursor: cursor and page label are read", () => {
    const p = parseListParams({ cursor: "c20", page: "2" }, STATUSES);
    expect(p.cursor).toBe("c20");
    expect(p.page).toBe(2);
  });

  it("Orders page unknown cursor shows the retry state: the cursor is passed through opaque", () => {
    expect(parseListParams({ cursor: "not-a-cursor" }, STATUSES).cursor).toBe(
      "not-a-cursor",
    );
    expect(parseListParams({ cursor: ["a", "b"] }, STATUSES).cursor).toBe("a");
  });

  it("Query is trimmed to 100 characters: 150 characters are cut to 100", () => {
    expect(parseListParams({ q: "y".repeat(150) }, STATUSES).q).toHaveLength(
      100,
    );
    expect(parseListParams({ q: "y".repeat(100) }, STATUSES).q).toHaveLength(
      100,
    );
    expect(parseListParams({ q: "y".repeat(99) }, STATUSES).q).toHaveLength(99);
  });

  it("Next page uses the returned cursor: buildListHref keeps cursor and page", () => {
    expect(
      buildListHref("/seller/orders", { cursor: "c20", page: 2 }),
    ).toBe("/seller/orders?cursor=c20&page=2");
  });

  it("Changing tab or search resets the position: an href built without cursor and page has neither", () => {
    const { cursor: _c, page: _p, ...rest } = parseListParams(
      { cursor: "c40", page: "3", q: "ab", status: "shipped" },
      STATUSES,
    );
    const href = buildListHref("/seller/orders", { ...rest, status: "draft" });
    expect(href).not.toContain("cursor");
    expect(href).not.toContain("page");
    expect(href).toBe("/seller/orders?q=ab&status=draft");
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

describe("parseMarketingParams", () => {
  it("Tab is kept in the URL: flashsale tab, cursor and status are read", () => {
    expect(
      parseMarketingParams({
        tab: "flashsale",
        cursor: "c20",
        status: "cancelled",
      }),
    ).toEqual({ tab: "flashsale", cursor: "c20", status: "cancelled" });
  });

  it("Unknown tab value falls back to the default: missing or bogus tab is voucher", () => {
    expect(parseMarketingParams({}).tab).toBe("voucher");
    expect(parseMarketingParams({ tab: "bogus" }).tab).toBe("voucher");
    expect(parseMarketingParams({ tab: ["flashsale", "x"] }).tab).toBe(
      "flashsale",
    );
  });

  it("Hub status filter narrows the list: only scheduled, active, ended, cancelled are accepted", () => {
    for (const s of ["scheduled", "active", "ended", "cancelled"]) {
      expect(parseMarketingParams({ status: s }).status).toBe(s);
    }
    expect(parseMarketingParams({ status: "bogus" }).status).toBe("all");
    expect(parseMarketingParams({}).status).toBe("all");
  });

  it("Unknown cursor shows the retry state: the cursor is passed through opaque and defaults to empty", () => {
    expect(parseMarketingParams({}).cursor).toBe("");
    expect(parseMarketingParams({ cursor: "!!bad" }).cursor).toBe("!!bad");
  });

  it("Tab is kept in the URL: buildListHref keeps tab and cursor and drops status=all", () => {
    expect(
      buildListHref("/seller/marketing", {
        tab: "flashsale",
        status: "all",
        cursor: "c1",
      }),
    ).toBe("/seller/marketing?tab=flashsale&cursor=c1");
  });
});
