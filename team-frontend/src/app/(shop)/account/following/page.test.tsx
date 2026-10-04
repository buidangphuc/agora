import { render, screen, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { listFollowedSellers } from "@/lib/gateway/engagement";
import { searchListings } from "@/lib/gateway/search";
import { getPrincipal } from "@/lib/gateway/session";

vi.mock("@/lib/gateway/session", () => ({ getPrincipal: vi.fn() }));
vi.mock("@/lib/gateway/engagement", () => ({ listFollowedSellers: vi.fn() }));
vi.mock("@/lib/gateway/search", () => ({ searchListings: vi.fn() }));

import FollowingPage from "./page";

// An async server component cannot render inside RTL; the feed is covered below.
vi.mock("@/features/account/FollowedFeed", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/features/account/FollowedFeed")>()),
  FollowedFeed: () => <p>feed-stub</p>,
}));

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(getPrincipal).mockReturnValue({ id: "u", name: "t", scopes: [] });
  vi.mocked(listFollowedSellers).mockResolvedValue([]);
});

describe("/account/following", () => {
  it("defaults to the shops tab, selected without client JS", async () => {
    render(await FollowingPage({}));
    const tabs = screen.getByRole("navigation", { name: "Tabs" });
    expect(
      within(tabs).getByRole("link", { name: /Gian hàng/ }),
    ).toHaveAttribute("aria-current", "page");
    expect(
      within(tabs).getByRole("link", { name: "Sản phẩm" }),
    ).not.toHaveAttribute("aria-current");
  });

  it("?tab=items selects the items tab and renders the feed", async () => {
    render(await FollowingPage({ searchParams: { tab: "items" } }));
    const tabs = screen.getByRole("navigation", { name: "Tabs" });
    expect(
      within(tabs).getByRole("link", { name: "Sản phẩm" }),
    ).toHaveAttribute("aria-current", "page");
    expect(
      within(tabs).getByRole("link", { name: "Sản phẩm" }),
    ).toHaveAttribute("href", "/account/following?tab=items");
    expect(screen.getByText("feed-stub")).toBeInTheDocument();
  });

  it("an unknown tab falls back to shops", async () => {
    render(await FollowingPage({ searchParams: { tab: "nope" } }));
    expect(
      screen.getByRole("link", { name: /Gian hàng/, current: "page" }),
    ).toBeInTheDocument();
  });

  it("shop cards show the real shop name and link to the shop", async () => {
    vi.mocked(listFollowedSellers).mockResolvedValue([
      { sellerId: "seller-aaaaaa-1", displayName: "Cửa hàng Hoa Mai" },
    ]);
    render(await FollowingPage({}));
    const link = screen.getByRole("link", { name: /Cửa hàng Hoa Mai/ });
    expect(link).toHaveAttribute("href", "/shop/seller-aaaaaa-1");
    expect(screen.queryByText(/Shop #/)).toBeNull();
  });

  it("an empty shop name falls back to Shop # and the first 6 characters", async () => {
    vi.mocked(listFollowedSellers).mockResolvedValue([
      { sellerId: "uvwxyz123456", displayName: "" },
    ]);
    render(await FollowingPage({}));
    expect(screen.getByText("Shop #uvwxyz")).toBeInTheDocument();
  });

  it("an empty follow list shows Empty with a link to /search", async () => {
    render(await FollowingPage({}));
    expect(
      screen.getByText("Bạn chưa theo dõi gian hàng nào."),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: "Khám phá gian hàng" }),
    ).toHaveAttribute("href", "/search");
  });
});

describe("FollowedFeed", () => {
  it("lists unique listings from the followed sellers linking to /listing/{id}", async () => {
    const feed = await vi.importActual<
      typeof import("@/features/account/FollowedFeed")
    >("@/features/account/FollowedFeed");
    vi.mocked(searchListings).mockImplementation(
      async (_q, f) =>
        ({
          items: [
            { id: "l1", title: "Áo thun" },
            { id: `l-${f?.sellerId}`, title: `Của ${f?.sellerId}` },
          ],
        }) as never,
    );
    render(await feed.FollowedFeed({ sellerIds: ["s1", "s2"] }));
    expect(screen.getAllByRole("listitem")).toHaveLength(3);
    expect(screen.getByRole("link", { name: "Áo thun" })).toHaveAttribute(
      "href",
      "/listing/l1",
    );
  });

  it("shows Empty when no listing is found", async () => {
    const feed = await vi.importActual<
      typeof import("@/features/account/FollowedFeed")
    >("@/features/account/FollowedFeed");
    vi.mocked(searchListings).mockResolvedValue({ items: [] } as never);
    render(await feed.FollowedFeed({ sellerIds: ["s1"] }));
    expect(
      screen.getByText("Bạn chưa theo dõi sản phẩm nào."),
    ).toBeInTheDocument();
  });

  it("has a skeleton with the same row count footprint as a populated feed", async () => {
    const feed = await vi.importActual<
      typeof import("@/features/account/FollowedFeed")
    >("@/features/account/FollowedFeed");
    render(<feed.FollowedFeedSkeleton />);
    const skeleton = screen.getByTestId("followed-feed-skeleton");
    expect(skeleton).toHaveAttribute("aria-busy", "true");
    expect(within(skeleton).getAllByRole("listitem")).toHaveLength(6);
  });
});
