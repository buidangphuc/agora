import { render } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const searchListings = vi.hoisted(() => vi.fn());
const view = vi.hoisted(() => vi.fn());

vi.mock("@/lib/gateway/search", () => ({ searchListings }));
const getStorefront = vi.hoisted(() => vi.fn());
const getListing = vi.hoisted(() => vi.fn());
vi.mock("@/lib/gateway/listings", () => ({
  getStorefront,
  getListing,
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

beforeEach(() => {
  vi.clearAllMocks();
  getStorefront.mockResolvedValue(null);
});

const sf = (ids: string[]) => ({
  sellerId: "s1",
  displayName: "",
  tagline: "",
  bannerUrl: "",
  featuredListingIds: ids,
});

async function renderPage() {
  render(
    await ShopPage({
      params: Promise.resolve({ id: "s1" }),
      searchParams: Promise.resolve({}),
    }),
  );
  return view.mock.calls[0][0] as { fetchedFeatured?: unknown[] };
}

describe("ShopPage", () => {
  it("lists only the shop's own listings through a seller-filtered search", async () => {
    searchListings.mockResolvedValue({ items: [{ id: "l1", sellerId: "s1" }] });
    render(
      await ShopPage({
        params: Promise.resolve({ id: "s1" }),
        searchParams: Promise.resolve({}),
      }),
    );
    expect(searchListings).toHaveBeenCalledWith("", { sellerId: "s1" });
    expect(view.mock.calls[0][0]).toMatchObject({
      sellerId: "s1",
      listings: [{ id: "l1", sellerId: "s1" }],
    });
  });

  it("never falls back to other shops' listings when the search fails", async () => {
    searchListings.mockRejectedValue(new Error("down"));
    render(
      await ShopPage({
        params: Promise.resolve({ id: "s1" }),
        searchParams: Promise.resolve({}),
      }),
    );
    expect(view.mock.calls[0][0]).toMatchObject({ listings: [] });
  });

  it("[Featured listings appear first in the chosen order] fetches only featured ids missing from the shop listing", async () => {
    searchListings.mockResolvedValue({ items: [{ id: "l1", sellerId: "s1" }] });
    getStorefront.mockResolvedValue(sf(["l2", "l1", "l3"]));
    getListing.mockImplementation(async (id: string) => ({
      id,
      sellerId: "s1",
      status: "published",
    }));
    const props = await renderPage();
    expect(getListing.mock.calls.map((c) => c[0]).sort()).toEqual([
      "l2",
      "l3",
    ]);
    expect(props.fetchedFeatured).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ id: "l2" }),
        expect.objectContaining({ id: "l3" }),
      ]),
    );
  });

  it("[Featured listings appear first in the chosen order] does not fetch when every featured id is already listed", async () => {
    searchListings.mockResolvedValue({ items: [{ id: "l1", sellerId: "s1" }] });
    getStorefront.mockResolvedValue(sf(["l1"]));
    const props = await renderPage();
    expect(getListing).not.toHaveBeenCalled();
    expect(props.fetchedFeatured).toEqual([]);
  });

  it("[A listing unpublished after being featured is skipped] a missing listing (null) is passed through for the view to skip", async () => {
    searchListings.mockResolvedValue({ items: [] });
    getStorefront.mockResolvedValue(sf(["gone"]));
    getListing.mockResolvedValue(null);
    const props = await renderPage();
    expect(getListing).toHaveBeenCalledWith("gone");
    expect(props.fetchedFeatured ?? []).not.toContainEqual(
      expect.objectContaining({ id: "gone" }),
    );
    expect(view).toHaveBeenCalledTimes(1);
  });

  it("[A storefront outage does not break the page] renders when a featured fetch rejects", async () => {
    searchListings.mockResolvedValue({ items: [{ id: "l1", sellerId: "s1" }] });
    getStorefront.mockResolvedValue(sf(["l9"]));
    getListing.mockRejectedValue(new Error("down"));
    const props = await renderPage();
    expect(getListing).toHaveBeenCalledWith("l9");
    expect(props.fetchedFeatured).toEqual([]);
    expect(view.mock.calls[0][0]).toMatchObject({
      listings: [{ id: "l1", sellerId: "s1" }],
    });
  });

  it("[A storefront outage does not break the page] makes no featured fetch without a storefront", async () => {
    searchListings.mockResolvedValue({ items: [] });
    getStorefront.mockResolvedValue(null);
    await renderPage();
    expect(getListing).not.toHaveBeenCalled();
  });
});
