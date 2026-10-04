import { render, screen, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { FABRICATED_PDP_TEXT } from "@/features/listing/fabricatedText";
import { getCategory, getListing, getStorefront } from "@/lib/gateway/listings";
import { getActiveFlashSale } from "@/lib/gateway/promotion";
import {
  getListingRatingSummary,
  getShopRatingSummary,
  listReviews,
} from "@/lib/gateway/reviews";
import ProductDetailPage from "./page";
import {
  makeListing,
  noVariantListing,
  rated,
  shopUnrated,
  unrated,
  variantListing,
} from "./pdpFixtures";

vi.mock("next/navigation", () => ({
  notFound: () => {
    throw new Error("NEXT_NOT_FOUND");
  },
  useRouter: () => ({ push: vi.fn(), replace: vi.fn() }),
  usePathname: () => "/listing/L",
  useSearchParams: () => new URLSearchParams(),
}));
vi.mock("@/lib/gateway/listings", () => ({
  getListing: vi.fn(),
  getCategory: vi.fn(),
  getStorefront: vi.fn(),
}));
vi.mock("@/lib/gateway/engagement", () => ({
  listCollections: vi.fn().mockResolvedValue([]),
  recordView: vi.fn().mockResolvedValue(undefined),
}));
vi.mock("@/lib/gateway/notification", () => ({
  AlertType: { PRICE_DROP: 1, BACK_IN_STOCK: 2 },
  listAlertSubscriptions: vi.fn().mockResolvedValue([]),
}));
vi.mock("@/lib/gateway/promotion", () => ({ getActiveFlashSale: vi.fn() }));
vi.mock("@/lib/gateway/reviews", () => ({
  getListingRatingSummary: vi.fn(),
  getShopRatingSummary: vi.fn(),
  listReviews: vi.fn(),
}));
vi.mock("@/lib/gateway/session", () => ({ getPrincipal: vi.fn(() => null) }));
vi.mock("@/features/cart/actions", () => ({ addToCartAction: vi.fn() }));
vi.mock("@/lib/analytics", () => ({ trackEcommerce: vi.fn() }));

// Client leaves with their own behaviour, and async server children that stream
// in their own Suspense boundaries, are stubbed: this file tests the page itself.
vi.mock("@/components/alerts/AlertToggle", () => ({ AlertToggle: () => null }));
vi.mock("@/features/engagement/AddToCollectionButton", () => ({
  AddToCollectionButton: () => null,
}));
vi.mock("@/features/engagement/FavoriteButton", () => ({
  FavoriteButton: () => null,
}));
vi.mock("@/features/listing/ShareButton", () => ({ ShareButton: () => null }));
vi.mock("@/features/chat/ChatWithSellerButton", () => ({
  ChatWithSellerButton: () => <button type="button">Chat Ngay</button>,
}));
vi.mock("@/features/listing/LiveFlashSaleStock", () => ({
  LiveFlashSaleStock: () => null,
}));
vi.mock("@/features/recommendations/RecommendationsRow", () => ({
  RecommendationsRow: () => null,
}));
vi.mock("@/features/review/ReviewList", () => ({ ReviewList: () => null }));
vi.mock("@/features/review/AiReviewSummary", () => ({
  AiReviewSummary: () => null,
}));
vi.mock("@/features/listing/qa/QuestionList", () => ({
  QuestionList: () => null,
}));

type Params = Parameters<typeof ProductDetailPage>[0];

async function renderPage(searchParams: Params["searchParams"] = {}) {
  return render(await ProductDetailPage({ params: { id: "L" }, searchParams }));
}

function setup(
  over: {
    listing?: ReturnType<typeof makeListing> | null;
    rating?: typeof unrated;
    flash?: Awaited<ReturnType<typeof getActiveFlashSale>>;
  } = {},
) {
  vi.mocked(getListing).mockResolvedValue(
    over.listing === undefined ? noVariantListing : over.listing,
  );
  vi.mocked(getCategory).mockResolvedValue({
    id: "cat1",
    name: "Điện thoại",
    slug: "dien-thoai",
    parentId: "",
    displayOrder: 0,
    iconUrl: "",
  });
  vi.mocked(getStorefront).mockResolvedValue({
    sellerId: "seller-abcdef123",
    slug: "hoa-mai",
    bannerUrl: "",
    tagline: "",
    featuredListingIds: [],
    theme: "",
    displayName: "Cửa hàng Hoa Mai",
  });
  vi.mocked(getListingRatingSummary).mockResolvedValue(over.rating ?? unrated);
  vi.mocked(getShopRatingSummary).mockResolvedValue(shopUnrated);
  vi.mocked(getActiveFlashSale).mockResolvedValue(
    over.flash ?? { active: false },
  );
  vi.mocked(listReviews).mockResolvedValue([]);
}

beforeEach(() => {
  vi.clearAllMocks();
  setup();
});

describe("ProductDetailPage: only real data", () => {
  it("shows no invented numbers for a listing with no reviews and no sale", async () => {
    const { container } = await renderPage();
    const ratingRow = screen.getByTestId("pdp-rating");
    expect(ratingRow).toHaveTextContent("Chưa có đánh giá");
    // No stars in the header rating row, no sold count, no compare-at price.
    expect(within(ratingRow).queryByRole("img")).toBeNull();
    const price = screen.getByTestId("pdp-price");
    expect(price.querySelector(".line-through")).toBeNull();
    expect(price).not.toHaveTextContent(/-\d+%/);
    expect(price).toHaveTextContent("1.000.000");
    for (const fake of FABRICATED_PDP_TEXT) {
      expect(container).not.toHaveTextContent(fake);
    }
    expect(container.querySelector('img[src*="unsplash"]')).toBeNull();
  });

  it("never guesses a Mall badge from the price or a brand keyword", async () => {
    setup({
      listing: makeListing({
        price: 6_000_000,
        title: "Apple Sony Nike Chính Hãng",
      }),
    });
    const { container } = await renderPage();
    expect(container).not.toHaveTextContent(/mall/i);
  });

  it("shows the rating with a Rate and the count when there are reviews", async () => {
    setup({ rating: rated });
    await renderPage();
    const ratingRow = screen.getByTestId("pdp-rating");
    expect(
      within(ratingRow).getByRole("img", { name: "4.2 trên 5 sao" }),
    ).toBeInTheDocument();
    expect(ratingRow).toHaveTextContent("4.2");
    expect(ratingRow).toHaveTextContent("12 đánh giá");
    expect(ratingRow).not.toHaveTextContent("Chưa có đánh giá");
  });

  it("shows the real flash-sale discount: regular price struck through, sale price, computed percent", async () => {
    setup({
      flash: {
        active: true,
        campaign: {
          id: "c1",
          salePrice: 800_000,
          stockCap: 10,
          stockSold: 2,
          remaining: 8,
        },
      } as never,
    });
    await renderPage();
    const price = screen.getByTestId("pdp-price");
    const struck = price.querySelector(".line-through");
    expect(struck).not.toBeNull();
    expect(struck).toHaveTextContent("1.000.000");
    expect(price).toHaveTextContent("800.000");
    expect(price).toHaveTextContent("-20%");
  });

  it("applies a variant-specific campaign only while that variant is shown", async () => {
    const flash = {
      active: true,
      campaign: {
        id: "c1",
        variantId: "v2",
        salePrice: 1_200_000,
        stockCap: 5,
        stockSold: 0,
        remaining: 5,
      },
    } as never;
    setup({ listing: variantListing, flash });
    const { unmount } = await renderPage({ variant: "v2" });
    expect(
      screen.getByTestId("pdp-price").querySelector(".line-through"),
    ).toHaveTextContent("1.500.000");
    expect(screen.getByTestId("pdp-price")).toHaveTextContent("1.200.000");
    unmount();
    await renderPage({ variant: "v1" });
    expect(
      screen.getByTestId("pdp-price").querySelector(".line-through"),
    ).toBeNull();
  });

  it("ignores a flash-sale price that is not below the regular price", async () => {
    setup({
      flash: {
        active: true,
        campaign: {
          id: "c1",
          salePrice: 1_200_000,
          stockCap: 1,
          stockSold: 0,
          remaining: 1,
        },
      } as never,
    });
    await renderPage();
    expect(
      screen.getByTestId("pdp-price").querySelector(".line-through"),
    ).toBeNull();
  });
});

describe("ProductDetailPage: anatomy", () => {
  it("has exactly one h1 with the title, in the order breadcrumb, header, shop card, anchor nav, sections", async () => {
    await renderPage();
    expect(screen.getAllByRole("heading", { level: 1 })).toHaveLength(1);
    expect(screen.getByRole("heading", { level: 1 })).toHaveTextContent(
      "Điện thoại Demo",
    );

    const order = [
      screen.getByRole("navigation", { name: "Breadcrumb" }),
      screen.getByRole("heading", { level: 1 }),
      screen.getByTestId("shop-header-card"),
      screen.getByRole("navigation", { name: "Nội dung sản phẩm" }),
      document.getElementById("specs") as HTMLElement,
      document.getElementById("reviews") as HTMLElement,
      document.getElementById("qa") as HTMLElement,
    ];
    for (let i = 0; i < order.length - 1; i++) {
      expect(
        order[i].compareDocumentPosition(order[i + 1]) &
          Node.DOCUMENT_POSITION_FOLLOWING,
      ).toBeTruthy();
    }
  });

  it("breadcrumb is Trang chủ > category > title", async () => {
    await renderPage();
    const crumbs = within(
      screen.getByRole("navigation", { name: "Breadcrumb" }),
    );
    expect(crumbs.getByRole("link", { name: "Trang chủ" })).toHaveAttribute(
      "href",
      "/",
    );
    expect(crumbs.getByRole("link", { name: "Điện thoại" })).toHaveAttribute(
      "href",
      "/search?category=cat1",
    );
    expect(crumbs.getByText("Điện thoại Demo")).toHaveAttribute(
      "aria-current",
      "page",
    );
  });

  it("renders stock and category in a Descriptions block followed by the description", async () => {
    await renderPage();
    const specs = document.getElementById("specs") as HTMLElement;
    const stock = within(specs).getByText("Kho hàng");
    expect(stock.tagName).toBe("DT");
    expect(stock.parentElement).toHaveTextContent("Kho hàng12 sản phẩm");
    expect(within(specs).getByText("Danh mục").parentElement).toHaveTextContent(
      "Điện thoại",
    );
    expect(specs).toHaveTextContent("Mô tả sản phẩm");
    // No invented rows.
    expect(specs).not.toHaveTextContent("Thương hiệu");
    expect(specs).not.toHaveTextContent("Gửi từ");
  });

  it("has the anchor nav with links matching the always-rendered section ids and no tabs", async () => {
    await renderPage();
    const nav = screen.getByRole("navigation", { name: "Nội dung sản phẩm" });
    const hrefs = within(nav)
      .getAllByRole("link")
      .map((a) => a.getAttribute("href"));
    expect(hrefs).toEqual(["#specs", "#reviews", "#qa"]);
    for (const id of ["specs", "reviews", "qa"]) {
      expect(document.getElementById(id)).not.toBeNull();
    }
    expect(screen.queryByRole("tablist")).toBeNull();
    expect(screen.queryByRole("tab")).toBeNull();
    // Sticky from lg only; below it the nav scrolls horizontally.
    expect(nav).toHaveClass("lg:sticky", "overflow-x-auto");
    expect(nav).not.toHaveClass("sticky");
  });

  it("renders the shop name from the shop-display-name contract in the shop card", async () => {
    await renderPage();
    expect(screen.getByTestId("shop-name")).toHaveTextContent(
      "Cửa hàng Hoa Mai",
    );
    expect(screen.getByRole("link", { name: "Xem Shop" })).toHaveAttribute(
      "href",
      "/shop/seller-abcdef123",
    );
  });

  it("does not wait for the reviews (they stream in their own boundaries)", async () => {
    vi.mocked(listReviews).mockReturnValue(new Promise(() => {}));
    await renderPage();
    expect(screen.getByRole("heading", { level: 1 })).toBeInTheDocument();
  });

  it("calls notFound only when the listing does not exist", async () => {
    setup({ listing: null });
    await expect(
      ProductDetailPage({ params: { id: "nope" }, searchParams: {} }),
    ).rejects.toThrow("NEXT_NOT_FOUND");
  });
});

describe("ProductDetailPage: variant from the URL", () => {
  it("shows the variant's price, stock and SKU in the first server render for ?variant=", async () => {
    setup({ listing: variantListing });
    await renderPage({ variant: "v2" });
    expect(screen.getByRole("radio", { name: "256GB" })).toBeChecked();
    expect(screen.getByTestId("pdp-price")).toHaveTextContent("1.500.000");
    const specs = document.getElementById("specs") as HTMLElement;
    expect(within(specs).getByText("Kho hàng").parentElement).toHaveTextContent(
      "3 sản phẩm",
    );
    expect(screen.getByTestId("pdp-sku")).toHaveTextContent("SKU-256");
    expect(screen.getByTestId("pdp-stock")).toHaveTextContent(
      "3 sản phẩm có sẵn",
    );
  });

  it("falls back to the first in-stock variant for an unknown id", async () => {
    setup({ listing: variantListing });
    await renderPage({ variant: "does-not-exist" });
    expect(screen.getByRole("radio", { name: "128GB" })).toBeChecked();
    expect(screen.getByTestId("pdp-price")).toHaveTextContent("1.000.000");
  });

  it("disables an out-of-stock variant with an Hết hàng tag", async () => {
    setup({ listing: variantListing });
    await renderPage();
    const gone = screen.getByRole("radio", { name: /512GB/ });
    expect(gone).toBeDisabled();
    expect(gone).toHaveAttribute("aria-disabled", "true");
  });
});
