import { render, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import type { ViewStorefront } from "@/lib/gateway/listings";
import type { ViewShopRatingSummary } from "@/lib/gateway/reviews";
import { ShopHeaderCard } from "./ShopHeaderCard";

const rated: ViewShopRatingSummary = {
  sellerId: "seller-abcdef123",
  averageRating: 4.6,
  reviewCount: 12,
  breakdown: { star1: 0, star2: 0, star3: 1, star4: 3, star5: 8 },
};
const unrated: ViewShopRatingSummary = {
  ...rated,
  averageRating: 5,
  reviewCount: 0,
};

function storefront(over: Partial<ViewStorefront> = {}): ViewStorefront {
  return {
    sellerId: "seller-abcdef123",
    slug: "hoa-mai",
    bannerUrl: "",
    tagline: "",
    featuredListingIds: [],
    theme: "",
    displayName: "Cửa hàng Hoa Mai",
    ...over,
  };
}

describe("ShopHeaderCard compact (PDP)", () => {
  it("shows the real shop name, the rating summary and a link to the storefront", () => {
    render(
      <ShopHeaderCard
        variant="compact"
        sellerId="seller-abcdef123"
        storefront={storefront()}
        summary={rated}
        actions={<button type="button">Chat Ngay</button>}
      />,
    );
    expect(screen.getByTestId("shop-name")).toHaveTextContent(
      "Cửa hàng Hoa Mai",
    );
    expect(screen.queryByText(/Shop #/)).toBeNull();
    const rating = screen.getByTestId("shop-rating-summary");
    expect(rating).toHaveTextContent("4.6 / 5.0");
    expect(rating).toHaveTextContent("12 đánh giá");
    expect(screen.getByRole("link", { name: "Xem Shop" })).toHaveAttribute(
      "href",
      "/shop/seller-abcdef123",
    );
    expect(
      screen.getByRole("button", { name: "Chat Ngay" }),
    ).toBeInTheDocument();
  });

  it("falls back to Shop # plus 6 characters for an empty or missing name", () => {
    const { rerender } = render(
      <ShopHeaderCard
        sellerId="seller-abcdef123"
        storefront={storefront({ displayName: "   " })}
        summary={rated}
      />,
    );
    expect(screen.getByTestId("shop-name")).toHaveTextContent("Shop #seller");
    rerender(
      <ShopHeaderCard
        sellerId="seller-abcdef123"
        storefront={null}
        summary={rated}
      />,
    );
    expect(screen.getByTestId("shop-name")).toHaveTextContent("Shop #seller");
  });

  it("shows Chưa có đánh giá, not 0.0 / 5.0, for a shop with no reviews", () => {
    render(
      <ShopHeaderCard
        sellerId="seller-abcdef123"
        storefront={storefront()}
        summary={unrated}
      />,
    );
    const rating = screen.getByTestId("shop-rating-summary");
    expect(rating).toHaveTextContent("Chưa có đánh giá");
    expect(rating).not.toHaveTextContent("/ 5.0");
    expect(rating).not.toHaveTextContent("0.0");
  });

  it("renders no invented response rate, tenure, Mall or online time", () => {
    const { container } = render(
      <ShopHeaderCard
        sellerId="seller-abcdef123"
        storefront={storefront()}
        summary={rated}
      />,
    );
    for (const fake of [
      "Tỉ Lệ Phản Hồi",
      "99%",
      "Mall",
      "Online",
      "Tham Gia",
    ]) {
      expect(container).not.toHaveTextContent(fake);
    }
  });
});

describe("ShopHeaderCard hero (storefront)", () => {
  it("renders the shop name as the h1, the banner and tagline, and real stats", () => {
    render(
      <ShopHeaderCard
        variant="hero"
        sellerId="seller-abcdef123"
        storefront={storefront({
          bannerUrl: "http://x/banner.png",
          tagline: "Hoa tươi mỗi ngày",
        })}
        summary={rated}
        productCount={7}
        actions={<button type="button">+ Theo Dõi</button>}
      />,
    );
    expect(
      screen.getByRole("heading", { level: 1, name: "Cửa hàng Hoa Mai" }),
    ).toBeInTheDocument();
    expect(screen.getByText("Hoa tươi mỗi ngày")).toBeInTheDocument();
    expect(screen.getByAltText("Ảnh bìa gian hàng")).toHaveAttribute(
      "src",
      "http://x/banner.png",
    );
    expect(
      screen.getByRole("button", { name: "+ Theo Dõi" }),
    ).toBeInTheDocument();
    const rating = screen.getByTestId("shop-rating-summary");
    expect(within(rating).getByText("4.6")).toBeInTheDocument();
    expect(screen.getByText("7")).toBeInTheDocument();
    // No "Xem Shop" link on the storefront itself.
    expect(screen.queryByRole("link", { name: "Xem Shop" })).toBeNull();
  });

  it("shows the empty rating and no banner/tagline when there is no data", () => {
    render(
      <ShopHeaderCard
        variant="hero"
        sellerId="seller-abcdef123"
        storefront={null}
        summary={unrated}
        productCount={0}
      />,
    );
    expect(screen.getByTestId("shop-rating-summary")).toHaveTextContent(
      "Chưa có đánh giá",
    );
    expect(screen.queryByAltText("Ảnh bìa gian hàng")).toBeNull();
    expect(
      screen.getByRole("heading", { level: 1, name: "Shop #seller" }),
    ).toBeInTheDocument();
  });
});
