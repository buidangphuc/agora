import { render } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const searchListings = vi.hoisted(() => vi.fn());
const view = vi.hoisted(() => vi.fn());

vi.mock("@/lib/gateway/search", () => ({ searchListings }));
vi.mock("@/lib/gateway/listings", () => ({
  getStorefront: vi.fn().mockResolvedValue(null),
  listBundlesBySeller: vi.fn().mockResolvedValue([]),
}));
vi.mock("@/lib/gateway/engagement", () => ({ isFollowing: vi.fn() }));
vi.mock("@/lib/gateway/reviews", () => ({
  getShopRatingSummary: vi.fn().mockResolvedValue(null),
}));
vi.mock("@/lib/gateway/session", () => ({ getPrincipal: () => null }));
vi.mock("@/features/shop/ShopStorefrontView", () => ({
  ShopStorefrontView: (props: unknown) => {
    view(props);
    return null;
  },
}));

import ShopPage from "./page";

beforeEach(() => vi.clearAllMocks());

describe("ShopPage", () => {
  it("lists only the shop's own listings through a seller-filtered search", async () => {
    searchListings.mockResolvedValue({ items: [{ id: "l1", sellerId: "s1" }] });
    render(await ShopPage({ params: { id: "s1" }, searchParams: {} }));
    expect(searchListings).toHaveBeenCalledWith("", { sellerId: "s1" });
    expect(view.mock.calls[0][0]).toMatchObject({
      sellerId: "s1",
      listings: [{ id: "l1", sellerId: "s1" }],
    });
  });

  it("never falls back to other shops' listings when the search fails", async () => {
    searchListings.mockRejectedValue(new Error("down"));
    render(await ShopPage({ params: { id: "s1" }, searchParams: {} }));
    expect(view.mock.calls[0][0]).toMatchObject({ listings: [] });
  });
});
