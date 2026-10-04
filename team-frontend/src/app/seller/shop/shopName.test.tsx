import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import ShopPage from "@/app/shop/[id]/page";
import { getStorefront } from "@/lib/gateway/listings";

// The seller sets the name on /seller/shop; the public shop page must show it.
vi.mock("@/features/shop/ShopStorefrontView", () => ({
  ShopStorefrontView: ({ shopName }: { shopName: string }) => (
    <h1>{shopName}</h1>
  ),
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
    render(await ShopPage({ params: { id: "seller-abcdef" } }));
    expect(
      screen.getByRole("heading", { name: "Nhà Sách An Nhiên" }),
    ).toBeInTheDocument();
  });

  it("falls back to Shop # + 6 characters when unset", async () => {
    vi.mocked(getStorefront).mockResolvedValue(null);
    render(await ShopPage({ params: { id: "seller-abcdef" } }));
    expect(
      screen.getByRole("heading", { name: "Shop #seller" }),
    ).toBeInTheDocument();
  });
});
