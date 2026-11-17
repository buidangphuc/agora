import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const searchListings = vi.hoisted(() => vi.fn());
const getPublicBadges = vi.hoisted(() => vi.fn());
const view = vi.hoisted(() => vi.fn());

vi.mock("@/lib/gateway/search", () => ({ searchListings }));
const getStorefront = vi.hoisted(() => vi.fn());
const getListing = vi.hoisted(() => vi.fn());
const listBundlesBySeller = vi.hoisted(() => vi.fn());
vi.mock("@/lib/gateway/listings", () => ({
  getStorefront,
  getListing,
  listBundlesBySeller,
}));
vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: vi.fn(), replace: vi.fn() }),
}));
vi.mock("@/features/cart/actions", () => ({ addBundleToCartAction: vi.fn() }));
vi.mock("@/lib/gateway/engagement", () => ({ isFollowing: vi.fn() }));
vi.mock("@/lib/gateway/reviews", () => ({
  getShopRatingSummary: vi.fn().mockResolvedValue(null),
}));
vi.mock("@/lib/gateway/verification", () => ({ getPublicBadges }));
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
  searchListings.mockResolvedValue({ items: [] });
  getPublicBadges.mockResolvedValue(new Map([["s1", false]]));
  getStorefront.mockResolvedValue(null);
  listBundlesBySeller.mockResolvedValue([]);
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
    expect(getListing.mock.calls.map((c) => c[0]).sort()).toEqual(["l2", "l3"]);
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

  const combo = {
    id: "b1",
    sellerId: "s1",
    title: "Combo hè",
    listingIds: ["l1", "l2"],
    bundlePrice: 120000,
    createdAt: "",
  };

  it("[C7] renders the combo card with the saving and an add button when all listings are published", async () => {
    searchListings.mockResolvedValue({
      items: [
        { id: "l1", sellerId: "s1", title: "Áo thun", price: 100000 },
        { id: "l2", sellerId: "s1", title: "Quần short", price: 50000 },
      ],
    });
    listBundlesBySeller.mockResolvedValue([combo]);
    await renderPage();
    expect(screen.getByText(/Tiết kiệm 30\.000 VND/)).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Thêm combo vào giỏ" }),
    ).toBeInTheDocument();
  });

  it("[C2] renders the combo as unavailable without a button when a listing is unpublished", async () => {
    searchListings.mockResolvedValue({
      items: [{ id: "l1", sellerId: "s1", title: "Áo thun", price: 100000 }],
    });
    listBundlesBySeller.mockResolvedValue([combo]);
    await renderPage();
    expect(screen.getByText("Combo tạm không khả dụng")).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Thêm combo vào giỏ" }),
    ).toBeNull();
  });
});

async function renderShop(id = "s1") {
  return render(
    await ShopPage({
      params: Promise.resolve({ id }),
      searchParams: Promise.resolve({}),
    }),
  );
}

describe("ShopPage verified badge", () => {
  it("[Verified seller's shop shows the badge] renders exactly 1 Đã xác minh badge", async () => {
    getPublicBadges.mockResolvedValue(new Map([["s1", true]]));
    await renderShop();
    expect(screen.getAllByText("Đã xác minh")).toHaveLength(1);
  });

  it("[Non-verified shops show no badge and no status] renders no badge and no status words", async () => {
    getPublicBadges.mockResolvedValue(new Map([["s1", false]]));
    const { container } = await renderShop();
    expect(screen.queryByText("Đã xác minh")).toBeNull();
    for (const w of ["Đang chờ duyệt", "Bị từ chối", "Chưa gửi hồ sơ"]) {
      expect(container.textContent).not.toContain(w);
    }
  });

  it("[Non-verified shops show no badge and no status] a seller missing from the response shows no badge", async () => {
    getPublicBadges.mockResolvedValue(new Map());
    await renderShop();
    expect(screen.queryByText("Đã xác minh")).toBeNull();
  });

  it("[Badge lookup failure does not break the page] still renders the storefront with 0 badges", async () => {
    getPublicBadges.mockRejectedValue(new Error("deadline exceeded"));
    await renderShop();
    expect(screen.queryByText("Đã xác minh")).toBeNull();
    expect(view).toHaveBeenCalledTimes(1);
  });

  it("[The badge call asks for the shop seller only] asks GetPublicBadges for the shop seller only", async () => {
    await renderShop("s9");
    expect(getPublicBadges).toHaveBeenCalledTimes(1);
    expect(getPublicBadges).toHaveBeenCalledWith(["s9"]);
  });
});
