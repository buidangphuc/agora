import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { ViewListing } from "@/lib/gateway/listings";

const loadFeed = vi.hoisted(() => vi.fn());
const getRecentlyViewed = vi.hoisted(() => vi.fn());
const listCategories = vi.hoisted(() => vi.fn());
const getListing = vi.hoisted(() => vi.fn());

const loadSponsored = vi.hoisted(() => vi.fn());

vi.mock("./data", () => ({ loadFeed }));
vi.mock("@/features/ads/data", () => ({ loadSponsored }));
vi.mock("@/lib/analytics", () => ({ trackEcommerce: vi.fn() }));
vi.mock("@/features/engagement/FavoriteButton", () => ({
  FavoriteButton: () => null,
}));
vi.mock("@/lib/gateway/engagement", () => ({ getRecentlyViewed }));
vi.mock("@/lib/gateway/listings", () => ({
  listCategories,
  getListing,
}));

import { CategoryGridBlock } from "./CategoryGridBlock";
import { FeedBlock } from "./FeedBlock";
import { FlashSaleBlock } from "./FlashSaleBlock";
import { Hero } from "./Hero";
import { RecentlyViewedRow } from "./RecentlyViewedRow";
import { ServiceHubs } from "./ServiceHubs";

function listing(id: string, price = 6000000): ViewListing {
  return {
    id,
    title: `Apple chính hãng ${id}`,
    description: "",
    price,
    currency: "VND",
    status: "published",
    sellerId: "s",
    imageKeys: [],
    imageUrl: "https://img.test/x.jpg",
    categoryId: "c",
    stock: 3,
    variants: [],
  };
}

beforeEach(() => {
  loadFeed.mockReset();
  loadSponsored.mockReset();
  loadSponsored.mockResolvedValue([]);
  getRecentlyViewed.mockReset();
  listCategories.mockReset();
  getListing.mockReset();
});

describe("FeedBlock", () => {
  it("shows an error Alert with a retry link when the feed fails", async () => {
    loadFeed.mockResolvedValue({ items: [], failed: true });
    render(await FeedBlock());
    expect(screen.getByRole("alert")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Thử lại" })).toHaveAttribute(
      "href",
      "/",
    );
  });

  it("shows the grid and no fabricated markers on real data", async () => {
    loadFeed.mockResolvedValue({
      items: [listing("a"), listing("b")],
      failed: false,
    });
    const { container } = render(await FeedBlock());
    expect(container.querySelectorAll('a[href^="/listing/"]').length).toBe(4);
    const text = container.textContent ?? "";
    expect(text).not.toMatch(/MALL|ĐÃ BÁN|Đã bán|★|-\d+%|FLASH/i);
    expect(container.querySelector(".line-through")).toBeNull();
    expect(
      screen.getByRole("link", { name: "Xem thêm gợi ý" }),
    ).toHaveAttribute("href", "/search");
  });

  it("shows an empty state, not an error, when there are no listings", async () => {
    loadFeed.mockResolvedValue({ items: [], failed: false });
    render(await FeedBlock());
    expect(screen.queryByRole("alert")).toBeNull();
    expect(
      screen.getByText("Hiện chưa có sản phẩm nào được đăng bán."),
    ).toBeInTheDocument();
  });
});

describe("FeedBlock sponsored slots", () => {
  it("Sponsored slot is shown on the home feed", async () => {
    loadFeed.mockResolvedValue({ items: [listing("a")], failed: false });
    loadSponsored.mockResolvedValue([
      { listing: listing("sp1"), impressionToken: "tok" },
    ]);
    render(await FeedBlock());
    expect(loadSponsored).toHaveBeenCalledWith("home", "");
    const slots = screen.getAllByTestId("sponsored-slot");
    expect(slots).toHaveLength(1);
    expect(screen.getByText("Tài trợ")).toBeInTheDocument();
  });

  it("Home dedupes the sponsored listing", async () => {
    loadFeed.mockResolvedValue({
      items: [listing("a"), listing("b")],
      failed: false,
    });
    loadSponsored.mockResolvedValue([
      { listing: listing("a"), impressionToken: "tok" },
    ]);
    const { container } = render(await FeedBlock());
    const slot = screen.getByTestId("sponsored-slot");
    const inSlot = slot.querySelectorAll('a[href^="/listing/a"]').length;
    expect(inSlot).toBeGreaterThan(0);
    expect(container.querySelectorAll('a[href^="/listing/a"]').length).toBe(
      inSlot,
    );
    expect(
      container.querySelectorAll('a[href^="/listing/b"]').length,
    ).toBeGreaterThan(0);
  });

  it("Home slot failure does not break the feed", async () => {
    loadFeed.mockResolvedValue({
      items: [listing("a"), listing("b")],
      failed: false,
    });
    loadSponsored.mockResolvedValue([]);
    const { container } = render(await FeedBlock());
    expect(screen.queryByTestId("sponsored-slot")).toBeNull();
    expect(screen.queryByRole("alert")).toBeNull();
    expect(container.querySelectorAll('a[href^="/listing/"]').length).toBe(4);
  });
});

describe("hidden blocks", () => {
  it("FlashSaleBlock renders nothing without a real campaign source", async () => {
    expect(await FlashSaleBlock()).toBeNull();
  });

  it("CategoryGridBlock hides on failure or empty", async () => {
    listCategories.mockRejectedValue(new Error("down"));
    expect(await CategoryGridBlock()).toBeNull();
    listCategories.mockResolvedValue([]);
    expect(await CategoryGridBlock()).toBeNull();
  });

  it("RecentlyViewedRow hides when there is no history", async () => {
    getRecentlyViewed.mockResolvedValue([]);
    expect(await RecentlyViewedRow({})).toBeNull();
    getRecentlyViewed.mockRejectedValue(new Error("down"));
    expect(await RecentlyViewedRow({})).toBeNull();
  });

  it("RecentlyViewedRow survives a listing-service outage (hides, never throws)", async () => {
    getRecentlyViewed.mockResolvedValue(["a", "b"]);
    getListing.mockRejectedValue(new Error("team-domain down"));
    expect(await RecentlyViewedRow({})).toBeNull();
  });

  it("RecentlyViewedRow keeps the listings that resolved when one lookup fails", async () => {
    getRecentlyViewed.mockResolvedValue(["a", "b"]);
    getListing.mockImplementation(async (id: string) => {
      if (id === "b") throw new Error("down");
      return listing(id);
    });
    render(await RecentlyViewedRow({}));
    expect(screen.getByText("Vừa xem")).toBeInTheDocument();
    expect(screen.getAllByText(/Apple chính hãng/)).toHaveLength(1);
  });
});

describe("static home blocks", () => {
  it("has exactly one primary call to action and no campaign claims", () => {
    const { container } = render(<Hero />);
    expect(screen.getByRole("link", { name: "Mua ngay" })).toHaveAttribute(
      "href",
      "/search",
    );
    expect(container.textContent).not.toMatch(/%|Hoàn xu|Trả góp|0Đ|50/);
    expect(container.innerHTML).not.toMatch(/text-\[/);
  });

  it("renders 8 neutral hub tiles with no rainbow colours or badges", () => {
    const { container } = render(<ServiceHubs />);
    expect(screen.getAllByRole("link")).toHaveLength(8);
    expect(container.innerHTML).not.toMatch(
      /(amber|emerald|orange|red|blue|yellow|purple|rose)-\d/,
    );
    const boxes = [...container.querySelectorAll('[aria-hidden="true"]')];
    expect(new Set(boxes.map((b) => b.className)).size).toBe(1);
  });
});
