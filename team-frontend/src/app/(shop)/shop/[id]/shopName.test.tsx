import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { getStorefront } from "@/lib/gateway/listings";
import { shopLabel } from "@/lib/gateway/shops";
import ShopPage from "./page";

// The seller sets the name on /seller/shop; the public shop page must show it.
// The real view renders ShopHeaderCard, which labels the shop with shopLabel().
vi.mock("@/features/shop/ShopStorefrontView", () => ({
  ShopStorefrontView: ({
    sellerId,
    storefront,
  }: {
    sellerId: string;
    storefront: { displayName?: string } | null;
  }) => <h1>{shopLabel(sellerId, storefront?.displayName)}</h1>,
}));
vi.mock("@/lib/gateway/reviews", () => ({
  getShopRatingSummary: vi.fn().mockResolvedValue(null),
}));
vi.mock("@/lib/gateway/engagement", () => ({ isFollowing: vi.fn() }));
vi.mock("@/lib/gateway/session", () => ({ getPrincipal: vi.fn(() => null) }));
vi.mock("@/lib/gateway/listings", () => ({
  getStorefront: vi.fn(),
  listBundlesBySeller: vi.fn().mockResolvedValue([]),
  listListings: vi
    .fn()
    .mockResolvedValue({ items: [], nextCursor: "", total: 0 }),
}));

beforeEach(() => vi.clearAllMocks());

describe("/shop/<id> shows the display name a seller saved", () => {
  it("renders the saved name", async () => {
    vi.mocked(getStorefront).mockResolvedValue({
      displayName: "Nhà Sách An Nhiên",
    } as never);
    render(
      await ShopPage({
        params: Promise.resolve({ id: "seller-abcdef" }),
        searchParams: Promise.resolve({}),
      }),
    );
    expect(
      screen.getByRole("heading", { name: "Nhà Sách An Nhiên" }),
    ).toBeInTheDocument();
  });

  it("falls back to Shop # + 6 characters when unset", async () => {
    vi.mocked(getStorefront).mockResolvedValue(null);
    render(
      await ShopPage({
        params: Promise.resolve({ id: "seller-abcdef" }),
        searchParams: Promise.resolve({}),
      }),
    );
    expect(
      screen.getByRole("heading", { name: "Shop #seller" }),
    ).toBeInTheDocument();
  });
});
