import { render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { ViewCartItem } from "@/lib/gateway/cart";

import { CartGroups } from "./CartGroups";
import { groupByShop } from "./groupByShop";

vi.mock("./actions", () => ({
  updateCartItemAction: vi.fn(),
  removeFromCartAction: vi.fn(),
}));

function item(
  id: string,
  sellerId: string,
  sellerDisplayName: string,
  over: Partial<ViewCartItem> = {},
): ViewCartItem {
  return {
    id,
    listingId: `l-${id}`,
    variantId: "",
    quantity: 2,
    unitPrice: 100000,
    title: `Sản phẩm ${id}`,
    variantName: "",
    imageUrl: `img-${id}.png`,
    sellerId,
    sellerDisplayName,
    ...over,
  };
}

describe("CartGroups", () => {
  it("renders one card per shop with only that shop's rows", () => {
    const groups = groupByShop([
      item("a", "seller-aaaaaa", "Cửa hàng Hoa Mai"),
      item("b", "seller-bbbbbb", "Shop Beta"),
      item("c", "seller-aaaaaa", "Cửa hàng Hoa Mai"),
    ]);
    render(<CartGroups groups={groups} />);
    const cards = screen.getAllByTestId("cart-shop-group");
    expect(cards).toHaveLength(2);
    expect(
      within(cards[0] as HTMLElement).getAllByRole("listitem"),
    ).toHaveLength(2);
    expect(
      within(cards[1] as HTMLElement).getAllByRole("listitem"),
    ).toHaveLength(1);
  });

  it("shows the real shop name linking to /shop/<sellerId>", () => {
    render(
      <CartGroups
        groups={groupByShop([item("a", "seller-aaaaaa", "Cửa hàng Hoa Mai")])}
      />,
    );
    const link = screen.getByRole("link", { name: "Cửa hàng Hoa Mai" });
    expect(link).toHaveAttribute("href", "/shop/seller-aaaaaa");
    expect(screen.queryByText(/Shop #/)).not.toBeInTheDocument();
  });

  it("falls back to Shop #<6 chars> only for an empty name", () => {
    render(
      <CartGroups groups={groupByShop([item("a", "zyxwvu123456", "")])} />,
    );
    expect(screen.getByRole("link", { name: "Shop #zyxwvu" })).toHaveAttribute(
      "href",
      "/shop/zyxwvu123456",
    );
  });

  it("loads the first group's thumbnails eagerly and later groups lazily", () => {
    const groups = groupByShop([
      item("a", "s1", "One"),
      item("b", "s2", "Two"),
      item("c", "s3", "Three"),
    ]);
    render(<CartGroups groups={groups} />);
    const cards = screen.getAllByTestId("cart-shop-group");
    const loadingOf = (card: HTMLElement | undefined) =>
      within(card as HTMLElement)
        .getByRole("img", { name: /Sản phẩm/ })
        .getAttribute("loading");
    expect(loadingOf(cards[0])).toBe("eager");
    expect(loadingOf(cards[1])).toBe("lazy");
    expect(loadingOf(cards[2])).toBe("lazy");
  });

  it("renders unit price, variant and the line total", () => {
    render(
      <CartGroups
        groups={groupByShop([
          item("a", "s1", "One", { variantName: "Đỏ", quantity: 3 }),
        ])}
      />,
    );
    expect(screen.getByText("Phân loại: Đỏ")).toBeInTheDocument();
    expect(screen.getByText("300.000")).toBeInTheDocument();
  });
});
