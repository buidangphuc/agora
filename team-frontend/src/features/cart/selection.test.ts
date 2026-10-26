import { randomUUID } from "node:crypto";
import { describe, expect, it } from "vitest";

import { parseItemsParam, resolveSelection, selectionHref } from "./selection";

describe("Cart selection: parsing trims, drops empties and de-duplicates", () => {
  it.each([
    [" a, b,,a ,c ", ["a", "b", "c"]],
    ["a", ["a"]],
    ["", []],
    [undefined, []],
    [",, ,", []],
    ["b,a,b,a", ["b", "a"]],
  ])("parse trims drops empties and dedupes: %j", (raw, want) => {
    expect(parseItemsParam(raw)).toEqual(want);
  });
});

describe("Cart selection: ids absent from the cart are ignored", () => {
  it("unknown ids are ignored and order follows the cart", () => {
    expect(resolveSelection(["l1", "l2", "l3"], "l3,zzz,l1")).toEqual([
      "l1",
      "l3",
    ]);
  });

  it("unknown ids are ignored with duplicates in the list", () => {
    expect(resolveSelection(["l1", "l2"], "l2,l2,ghost,l2")).toEqual(["l2"]);
  });
});

describe("Cart selection: an empty or unmatched list resolves to no line", () => {
  it.each([
    [["l1", "l2"], ""],
    [["l1", "l2"], "ghost"],
    [["l1", "l2"], "ghost1,ghost2"],
    [[], "l1"],
  ])("empty or unmatched resolves to nothing: %j %j", (cart, raw) => {
    expect(resolveSelection(cart, raw)).toEqual([]);
  });
});

describe("Cart selection: a 50 line selection fits one link", () => {
  it("fifty ids round trip", () => {
    const ids = Array.from({ length: 50 }, () => randomUUID());
    const href = selectionHref(ids);
    expect(href.length).toBeLessThanOrEqual(2200);
    expect(href.startsWith("/checkout?")).toBe(true);
    const items = new URL(href, "http://x").searchParams.get("items");
    expect(parseItemsParam(items ?? undefined)).toEqual(ids);
  });

  it("encodes commas as %2C like URLSearchParams", () => {
    expect(selectionHref(["l1", "l3"])).toBe("/checkout?items=l1%2Cl3");
  });

  it("extra params are appended and items survive", () => {
    const href = selectionHref(["l1", "l3"], { step: "payment" });
    const q = new URL(href, "http://x").searchParams;
    expect(q.get("step")).toBe("payment");
    expect(parseItemsParam(q.get("items") ?? undefined)).toEqual(["l1", "l3"]);
  });
});

describe("Cart selection: a line id that is not the buyer's own resolves to nothing", () => {
  it("foreign line id resolves to nothing", () => {
    expect(resolveSelection(["l1", "l2"], "y9")).toEqual([]);
  });
});

describe("Cart selection: an items parameter with no current line ticks everything", () => {
  it("no valid id falls back to all lines for the cart page", () => {
    const cart = ["l1", "l2", "l3"];
    const resolved = resolveSelection(cart, "ghost");
    expect(resolved).toEqual([]);
    expect(resolved.length === 0 ? cart : resolved).toEqual(cart);
  });
});
