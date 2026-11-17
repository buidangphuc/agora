import { describe, expect, it } from "vitest";

import type { ViewListing } from "@/lib/gateway/listings";

import { dedupe } from "./dedupe";

function listing(id: string): ViewListing {
  return {
    id,
    title: id,
    description: "",
    price: 1,
    currency: "VND",
    status: "published",
    sellerId: "s",
    imageKeys: [],
    categoryId: "c",
    stock: 1,
    variants: [],
  };
}

const sponsored = (id: string) => ({
  listing: listing(id),
  impressionToken: `tok-${id}`,
});

describe("dedupe", () => {
  it("removes sponsored listings from organic items and keeps order", () => {
    const items = [listing("a"), listing("b"), listing("c"), listing("d")];

    const out = dedupe(items, [sponsored("b"), sponsored("d")]);

    expect(out.map((l) => l.id)).toEqual(["a", "c"]);
  });

  it("returns all items when there are no sponsored slots", () => {
    const items = [listing("a"), listing("b")];

    expect(dedupe(items, []).map((l) => l.id)).toEqual(["a", "b"]);
  });

  it("ignores sponsored listings absent from the organic items", () => {
    const items = [listing("a")];

    expect(dedupe(items, [sponsored("z")]).map((l) => l.id)).toEqual(["a"]);
  });

  it("does not mutate the input array", () => {
    const items = [listing("a"), listing("b")];

    dedupe(items, [sponsored("a")]);

    expect(items).toHaveLength(2);
  });
});
