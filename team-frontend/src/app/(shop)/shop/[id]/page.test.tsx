import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const searchListings = vi.hoisted(() => vi.fn());
const getPublicBadges = vi.hoisted(() => vi.fn());
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
});

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
