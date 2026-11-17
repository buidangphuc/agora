import { render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { ViewListing, ViewStorefront } from "@/lib/gateway/listings";
import { ShopStorefrontView, sortListings } from "./ShopStorefrontView";

vi.mock("@/features/listing/ListingGrid", () => ({
  ListingGrid: ({ listings }: { listings: ViewListing[] }) => (
    <ol data-testid="grid">
      {listings.map((l) => (
        <li key={l.id}>{l.title}</li>
      ))}
    </ol>
  ),
}));
vi.mock("@/features/chat/ChatWithSellerButton", () => ({
  ChatWithSellerButton: () => <button type="button">Chat Ngay</button>,
}));
vi.mock("@/features/engagement/FollowSellerButton", () => ({
  FollowSellerButton: () => <button type="button">+ Theo Dõi</button>,
}));

function listing(id: string, price: number): ViewListing {
  return {
    id,
    title: `L${id}`,
    description: "",
    price,
    currency: "VND",
    status: "published",
    sellerId: "s1",
    imageKeys: [],
    categoryId: "",
    stock: 1,
    variants: [],
  };
}

const listings = [listing("1", 300), listing("2", 100), listing("3", 200)];
const summary = {
  sellerId: "s1",
  averageRating: 0,
  reviewCount: 0,
  breakdown: { star1: 0, star2: 0, star3: 0, star4: 0, star5: 0 },
};

const order = () =>
  within(screen.getByTestId("grid"))
    .getAllByRole("listitem")
    .map((li) => li.textContent);

describe("sortListings", () => {
  it("orders by price for the two price sorts and leaves the rest as returned", () => {
    expect(sortListings(listings, "price_asc").map((l) => l.id)).toEqual([
      "2",
      "3",
      "1",
    ]);
    expect(sortListings(listings, "price_desc").map((l) => l.id)).toEqual([
      "1",
      "3",
      "2",
    ]);
    expect(sortListings(listings, "all").map((l) => l.id)).toEqual([
      "1",
      "2",
      "3",
    ]);
  });
});

describe("ShopStorefrontView", () => {
  it("orders the grid by ?sort=price_asc and marks that tab as the current link", () => {
    render(
      <ShopStorefrontView
        sellerId="s1"
        listings={listings}
        loggedIn
        summary={summary}
        sort="price_asc"
      />,
    );
    expect(order()).toEqual(["L2", "L3", "L1"]);
    const current = screen.getByRole("link", { name: "Giá: Thấp đến Cao" });
    expect(current).toHaveAttribute("aria-current", "page");
    expect(current).toHaveAttribute("href", "/shop/s1?sort=price_asc");
    expect(
      screen.getByRole("link", { name: "Giá: Cao đến Thấp" }),
    ).toHaveAttribute("href", "/shop/s1?sort=price_desc");
    expect(
      screen.getByRole("link", { name: "Tất cả sản phẩm (3)" }),
    ).toHaveAttribute("href", "/shop/s1");
  });

  it("keeps the listing order by default and shows the shared header with its actions", () => {
    render(
      <ShopStorefrontView
        sellerId="s1"
        listings={listings}
        loggedIn={false}
        summary={summary}
      />,
    );
    expect(order()).toEqual(["L1", "L2", "L3"]);
    expect(
      screen.getByRole("link", { name: "Tất cả sản phẩm (3)" }),
    ).toHaveAttribute("aria-current", "page");
    expect(screen.getByTestId("shop-header-card")).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "+ Theo Dõi" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Chat Ngay" }),
    ).toBeInTheDocument();
  });
});

function sf(over: Partial<ViewStorefront> = {}): ViewStorefront {
  return {
    sellerId: "s1",
    slug: "s",
    bannerUrl: "",
    tagline: "",
    featuredListingIds: [],
    theme: "",
    displayName: "Shop",
    ...over,
  };
}

const featuredTitles = () =>
  within(screen.getByTestId("featured-listings"))
    .getAllByRole("listitem")
    .map((li) => li.textContent);

describe("ShopStorefrontView featured block", () => {
  it("[Featured listings appear first in the chosen order] block lists 3,1 in stored order and the grid excludes them", () => {
    render(
      <ShopStorefrontView
        sellerId="s1"
        listings={listings}
        loggedIn
        summary={summary}
        storefront={sf({ featuredListingIds: ["3", "1"] })}
      />,
    );
    expect(featuredTitles()).toEqual(["L3", "L1"]);
    expect(order()).toEqual(["L2"]);
    expect(
      screen.getByRole("link", { name: "Tất cả sản phẩm (3)" }),
    ).toBeInTheDocument();
  });

  it("[No featured listings means no block] absent when the storefront has no featured ids", () => {
    render(
      <ShopStorefrontView
        sellerId="s1"
        listings={listings}
        loggedIn
        summary={summary}
        storefront={sf()}
      />,
    );
    expect(screen.queryByTestId("featured-listings")).toBeNull();
    expect(order()).toEqual(["L1", "L2", "L3"]);
  });

  it("[No featured listings means no block] absent when every featured id is unavailable", () => {
    render(
      <ShopStorefrontView
        sellerId="s1"
        listings={listings}
        loggedIn
        summary={summary}
        storefront={sf({ featuredListingIds: ["gone", "nope"] })}
      />,
    );
    expect(screen.queryByTestId("featured-listings")).toBeNull();
    expect(order()).toEqual(["L1", "L2", "L3"]);
  });

  it("[A storefront outage does not break the page] renders without a storefront", () => {
    render(
      <ShopStorefrontView
        sellerId="s1"
        listings={listings}
        loggedIn
        summary={summary}
        storefront={null}
      />,
    );
    expect(screen.queryByTestId("featured-listings")).toBeNull();
    expect(order()).toEqual(["L1", "L2", "L3"]);
    expect(screen.getByTestId("shop-header-card")).toHaveAttribute(
      "data-theme",
      "default",
    );
  });
});
