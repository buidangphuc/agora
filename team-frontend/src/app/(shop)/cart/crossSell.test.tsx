import { render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { isCheckoutEnabled } from "@/lib/flags";
import type { ViewCart, ViewCartItem } from "@/lib/gateway/cart";
import { getCartWithShopNames } from "@/lib/gateway/cart";
import { listVouchers, previewVoucher } from "@/lib/gateway/promotion";
import { getRecommendations } from "@/lib/gateway/recommendations";
import { getPrincipal } from "@/lib/gateway/session";
import {
  beaconsOf,
  beginBeaconCapture,
  drain,
  endBeaconCapture,
  viewListing,
} from "@/test/listBeacons";

import CartPage from "./page";

vi.mock("@/lib/flags", () => ({ isCheckoutEnabled: vi.fn() }));
vi.mock("@/lib/gateway/cart", () => ({ getCartWithShopNames: vi.fn() }));
vi.mock("@/lib/gateway/promotion", () => ({
  listVouchers: vi.fn(),
  previewVoucher: vi.fn(),
}));
vi.mock("@/lib/gateway/recommendations", () => ({
  getRecommendations: vi.fn(),
  RecommendationContext: { HOMEPAGE: 1, SIMILAR_ITEMS: 2, CART: 3 },
}));
vi.mock("@/features/cart/actions", () => ({
  updateCartItemAction: vi.fn(),
  removeFromCartAction: vi.fn(),
  clearCartAction: vi.fn(),
}));
vi.mock("@/features/voucher/actions", () => ({
  previewVoucherAction: vi.fn(),
}));
vi.mock("@/features/engagement/FavoriteButton", () => ({
  FavoriteButton: () => null,
}));
vi.mock("@/lib/gateway/session", () => ({ getPrincipal: vi.fn() }));
vi.mock("next/navigation", () => ({
  redirect: vi.fn((url: string) => {
    throw new Error(`NEXT_REDIRECT:${url}`);
  }),
  useRouter: () => ({ replace: vi.fn() }),
  usePathname: () => "/cart",
  useSearchParams: () => new URLSearchParams(),
}));
vi.mock("@/features/recommendations/CartCrossSellRow", () => ({
  CartCrossSellRow: (props: { lines: { listingId: string }[] }) => (
    <div
      data-testid="cross-sell-stub"
      data-listing-ids={props.lines.map((l) => l.listingId).join(",")}
    />
  ),
}));

const { CartCrossSellRow } = await vi.importActual<
  typeof import("@/features/recommendations/CartCrossSellRow")
>("@/features/recommendations/CartCrossSellRow");

function item(n: number, listingId = `l-${n}`): ViewCartItem {
  return {
    id: `i${n}`,
    listingId,
    variantId: "",
    quantity: 1,
    unitPrice: 100000,
    title: `Sản phẩm ${n}`,
    variantName: "",
    imageUrl: "",
    sellerId: "s1",
    sellerDisplayName: "Shop",
  };
}

function cartOf(items: ViewCartItem[]): ViewCart {
  return {
    userId: "u1",
    items,
    subtotal: items.reduce((a, i) => a + i.unitPrice * i.quantity, 0),
    totalItems: items.length,
  };
}

const lines = (n: number) => Array.from({ length: n }, (_, i) => item(i + 1));

const recs = (n: number) => ({
  items: Array.from({ length: n }, (_, i) => viewListing(`r${i + 1}`)),
  requestId: "req-cart-1",
  placementId: "cart_cross_sell",
  modelVersion: "m-cart-1",
});

let fetchMock: ReturnType<typeof vi.fn>;
beforeEach(() => {
  vi.clearAllMocks();
  fetchMock = beginBeaconCapture();
  vi.mocked(getPrincipal).mockReturnValue({ id: "u1" } as never);
  vi.mocked(isCheckoutEnabled).mockResolvedValue(true);
  vi.mocked(listVouchers).mockResolvedValue([]);
  vi.mocked(previewVoucher).mockResolvedValue({
    valid: false,
    reason: "",
    discountAmount: 0,
    voucherId: "",
  });
  vi.mocked(getRecommendations).mockResolvedValue(recs(3));
});
afterEach(endBeaconCapture);

async function renderPage() {
  render(await CartPage({ searchParams: Promise.resolve({}) }));
}

const requestOf = () => vi.mocked(getRecommendations).mock.calls[0]?.[0];

describe("/cart page wires the cross-sell row", () => {
  it("[Row shows for a cart with items] page passes the cart lines in cart order to the row", async () => {
    vi.mocked(getCartWithShopNames).mockResolvedValue(cartOf(lines(3)));
    await renderPage();
    expect(screen.getByTestId("cross-sell-stub")).toHaveAttribute(
      "data-listing-ids",
      "l-1,l-2,l-3",
    );
  });

  it("[Row is hidden for an empty cart] page renders no row for an empty cart", async () => {
    vi.mocked(getCartWithShopNames).mockResolvedValue(cartOf([]));
    await renderPage();
    expect(screen.queryByTestId("cross-sell-stub")).not.toBeInTheDocument();
    expect(getRecommendations).not.toHaveBeenCalled();
  });
});

describe("CartCrossSellRow", () => {
  it("[Row shows for a cart with items] renders the Thường được mua cùng heading with at most 8 listings from placement cart_cross_sell", async () => {
    vi.mocked(getRecommendations).mockResolvedValue(recs(12));
    const { container } = render(
      await CartCrossSellRow({ lines: lines(2) }),
    );
    expect(screen.getByText("Thường được mua cùng")).toBeInTheDocument();
    expect(
      container.querySelectorAll('a[href^="/listing/r"]').length,
    ).toBeLessThanOrEqual(8);
    expect(container.querySelectorAll('a[href^="/listing/r"]')).not.toHaveLength(
      0,
    );
    expect(requestOf()).toMatchObject({ context: 3 });
    expect(requestOf()?.limit).toBeLessThanOrEqual(8);
    expect(requestOf()?.limit).toBeGreaterThan(0);
  });

  it("[Row shows for a cart with items] sends the distinct listing ids of the cart lines as contextListingIds", async () => {
    render(
      await CartCrossSellRow({
        lines: [item(1, "a"), item(2, "b"), item(3, "a")],
      }),
    );
    expect(requestOf()?.contextListingIds).toEqual(["a", "b"]);
  });

  it("[At most 50 lines are sent] 60 lines send exactly the last 50 listing ids in cart order", async () => {
    render(await CartCrossSellRow({ lines: lines(60) }));
    const ids = requestOf()?.contextListingIds;
    expect(ids).toHaveLength(50);
    expect(ids).toEqual(Array.from({ length: 50 }, (_, i) => `l-${i + 11}`));
  });

  it("[At most 50 lines are sent] 50 lines are all sent and 51 lines drop only the oldest", async () => {
    render(await CartCrossSellRow({ lines: lines(50) }));
    expect(requestOf()?.contextListingIds).toEqual(
      Array.from({ length: 50 }, (_, i) => `l-${i + 1}`),
    );
    vi.mocked(getRecommendations).mockClear();
    render(await CartCrossSellRow({ lines: lines(51) }));
    expect(requestOf()?.contextListingIds).toEqual(
      Array.from({ length: 50 }, (_, i) => `l-${i + 2}`),
    );
  });

  it("[At most 50 lines are sent] the 50-line window is taken before de-duplication", async () => {
    const many = [
      item(0, "old"),
      ...Array.from({ length: 49 }, (_, i) => item(i + 1, "dup")),
      item(50, "new"),
    ];
    render(await CartCrossSellRow({ lines: many }));
    expect(requestOf()?.contextListingIds).toEqual(["dup", "new"]);
  });

  it("[Row is hidden for an empty cart] renders nothing and makes no Recommend call for an empty cart", async () => {
    expect(await CartCrossSellRow({ lines: [] })).toBeNull();
    expect(getRecommendations).not.toHaveBeenCalled();
  });

  it("[Row is hidden for an empty result] renders nothing when Recommend returns zero items", async () => {
    vi.mocked(getRecommendations).mockResolvedValue(recs(0));
    expect(await CartCrossSellRow({ lines: lines(2) })).toBeNull();
  });

  it("[Cart renders when recommendations fail] the row renders nothing and the cart page still renders when the call rejects", async () => {
    vi.mocked(getRecommendations).mockRejectedValue(new Error("UNAVAILABLE"));
    expect(await CartCrossSellRow({ lines: lines(2) })).toBeNull();
    vi.mocked(getCartWithShopNames).mockResolvedValue(cartOf(lines(2)));
    await renderPage();
    expect(screen.getAllByTestId("cart-shop-group")).toHaveLength(1);
    expect(screen.getByTestId("order-total")).toHaveTextContent("200.000");
  });

  it("[Beacons carry the recommendation identity] impression beacons carry impressionId, modelVersion and placementId", async () => {
    render(await CartCrossSellRow({ lines: lines(2) }));
    await drain();
    const beacons = beaconsOf(fetchMock);
    expect(beacons.length).toBeGreaterThan(0);
    for (const b of beacons) {
      expect(b).toMatchObject({
        impressionId: "req-cart-1",
        modelVersion: "m-cart-1",
        placementId: "cart_cross_sell",
      });
    }
  });

  it("[Beacons carry the recommendation identity] placement falls back to cart_cross_sell when the server reports none", async () => {
    vi.mocked(getRecommendations).mockResolvedValue({
      ...recs(2),
      placementId: "",
    });
    render(await CartCrossSellRow({ lines: lines(2) }));
    await drain();
    const beacons = beaconsOf(fetchMock);
    expect(beacons.length).toBeGreaterThan(0);
    for (const b of beacons) {
      expect(b).toMatchObject({ placementId: "cart_cross_sell" });
    }
  });
});
