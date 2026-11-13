import { describe, expect, it } from "vitest";
import type { ViewListing, ViewStorefront } from "@/lib/gateway/listings";
import { resolveFeatured } from "./featured";

const SHOP = "seller-1";

function listing(id: string, over: Partial<ViewListing> = {}): ViewListing {
  return {
    id,
    title: `Listing ${id}`,
    description: "",
    price: 1000,
    currency: "VND",
    status: "published",
    sellerId: SHOP,
    imageKeys: [],
    categoryId: "cat",
    stock: 1,
    variants: [],
    ...over,
  };
}

function storefront(ids: string[]): ViewStorefront {
  return {
    sellerId: SHOP,
    slug: "s",
    bannerUrl: "",
    tagline: "",
    featuredListingIds: ids,
    theme: "default",
    displayName: "",
  };
}

const ids = (xs: ViewListing[]) => xs.map((l) => l.id);

describe("resolveFeatured", () => {
  it("keeps stored order (scenario: Featured listings appear first in the chosen order)", () => {
    const shop = [listing("A"), listing("B"), listing("C"), listing("D")];
    const out = resolveFeatured(storefront(["B", "A", "C"]), shop, []);
    expect(ids(out)).toEqual(["B", "A", "C"]);
  });

  it("reuses shop listings and fetched ones in stored order (scenario: Featured listings appear first in the chosen order)", () => {
    const out = resolveFeatured(
      storefront(["X", "A", "Y"]),
      [listing("A")],
      [listing("Y"), listing("X")],
    );
    expect(ids(out)).toEqual(["X", "A", "Y"]);
  });

  it("skips unpublished and missing listings (scenario: A listing unpublished after being featured is skipped)", () => {
    const shop = [listing("A"), listing("C")];
    const out = resolveFeatured(
      storefront(["A", "B", "C", "GONE"]),
      shop,
      [listing("B", { status: "draft" }), null],
    );
    expect(ids(out)).toEqual(["A", "C"]);
  });

  it("treats an unset status as published (scenario: A listing unpublished after being featured is skipped)", () => {
    const out = resolveFeatured(storefront(["A"]), [listing("A", { status: "" })], []);
    expect(ids(out)).toEqual(["A"]);
  });

  it("drops a listing owned by another seller (scenario: A featured id of another shop is not shown)", () => {
    const shop = [listing("A")];
    const out = resolveFeatured(storefront(["A", "Z"]), shop, [
      listing("Z", { sellerId: "someone-else" }),
    ]);
    expect(ids(out)).toEqual(["A"]);
  });

  it("caps at 6 and dedupes (scenario: At most 6 featured listings are shown)", () => {
    const all = ["1", "2", "3", "4", "5", "6", "7", "8"].map((i) => listing(i));
    const out = resolveFeatured(
      storefront(["1", "1", "2", "3", "4", "5", "6", "7", "8"]),
      all,
      [],
    );
    expect(ids(out)).toEqual(["1", "2", "3", "4", "5", "6"]);
  });

  it("returns an empty list without a storefront or featured ids (scenario: At most 6 featured listings are shown)", () => {
    expect(resolveFeatured(null, [listing("A")], [])).toEqual([]);
    expect(resolveFeatured(storefront([]), [listing("A")], [])).toEqual([]);
  });
});
