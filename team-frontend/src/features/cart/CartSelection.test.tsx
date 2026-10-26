import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import type { ViewCartItem } from "@/lib/gateway/cart";

import { CartGroups } from "./CartGroups";
import { CartSelection } from "./CartSelection";
import { groupByShop } from "./groupByShop";

vi.mock("./actions", () => ({
  updateCartItemAction: vi.fn(),
  removeFromCartAction: vi.fn(),
  clearCartAction: vi.fn(),
}));
vi.mock("next/navigation", () => ({
  useRouter: () => ({ replace: vi.fn(), push: vi.fn(), refresh: vi.fn() }),
  usePathname: () => "/cart",
  useSearchParams: () => new URLSearchParams(),
}));

function item(
  id: string,
  sellerId: string,
  unitPrice: number,
  quantity = 1,
): ViewCartItem {
  return {
    id,
    listingId: `l-${id}`,
    variantId: "",
    quantity,
    unitPrice,
    title: `Sản phẩm ${id}`,
    variantName: "",
    imageUrl: "",
    sellerId,
    sellerDisplayName: `Shop ${sellerId}`,
  };
}

const ITEMS = [
  item("A", "s1", 300000),
  item("B", "s1", 200000),
  item("C", "s2", 100000, 2),
];

interface Over {
  items?: ViewCartItem[];
  initialItems?: string;
  checkoutEnabled?: boolean;
  discount?: number;
  voucherCode?: string;
}

function setup(over: Over = {}) {
  const items = over.items ?? ITEMS;
  const groups = groupByShop(items);
  render(
    <CartSelection
      lines={items.map((i) => ({
        id: i.id,
        sellerId: i.sellerId,
        unitPrice: i.unitPrice,
        quantity: i.quantity,
      }))}
      initialItems={over.initialItems}
      checkoutEnabled={over.checkoutEnabled ?? true}
      discount={over.discount ?? 0}
      voucherCode={over.voucherCode}
    >
      <CartGroups groups={groups} />
    </CartSelection>,
  );
}

const line = (id: string) =>
  screen.getByTestId(`cart-line-checkbox-${id}`) as HTMLInputElement;
const shop = (id: string) =>
  screen.getByTestId(`cart-shop-checkbox-${id}`) as HTMLInputElement;
const buyLinks = () => screen.queryAllByRole("link", { name: "Mua hàng" });
const buyHref = () => buyLinks()[0]?.getAttribute("href");

describe("CartSelection island", () => {
  it("[Cart selection: every line is ticked by default] all lines ticked by default", () => {
    setup();
    for (const id of ["A", "B", "C"]) expect(line(id).checked).toBe(true);
    expect(shop("s1").checked).toBe(true);
    expect(shop("s2").checked).toBe(true);
    expect(buyHref()).toBe("/checkout?items=A%2CB%2CC");
    expect(screen.getByText(/Tạm tính \(3 sản phẩm\)/)).toBeInTheDocument();
  });

  it("[Cart selection: ticking a subset changes the link and the summary] subset updates href and summary", async () => {
    setup();
    const user = userEvent.setup();
    await user.click(line("B"));
    await user.click(line("C"));
    expect(line("A").checked).toBe(true);
    expect(line("B").checked).toBe(false);
    expect(line("C").checked).toBe(false);
    expect(buyHref()).toBe("/checkout?items=A");
    expect(screen.getByText(/Tạm tính \(1 sản phẩm\)/)).toBeInTheDocument();
    expect(screen.getByTestId("order-total")).toHaveTextContent("300.000");
  });

  it("[Cart selection: ticking a subset changes the link and the summary] link keeps cart order whatever the tick order", async () => {
    setup();
    const user = userEvent.setup();
    await user.click(line("A"));
    await user.click(line("B"));
    await user.click(line("C"));
    await user.click(line("C"));
    await user.click(line("A"));
    expect(buyHref()).toBe("/checkout?items=A%2CC");
  });

  it("[Cart selection: a shop select-all toggles that shop's lines] shop select all toggles its lines", async () => {
    setup();
    const user = userEvent.setup();
    await user.click(shop("s1"));
    expect(line("A").checked).toBe(false);
    expect(line("B").checked).toBe(false);
    expect(line("C").checked).toBe(true);
    expect(shop("s1").checked).toBe(false);
    expect(buyHref()).toBe("/checkout?items=C");
    await user.click(shop("s1"));
    expect(line("A").checked).toBe(true);
    expect(line("B").checked).toBe(true);
    expect(line("C").checked).toBe(true);
    expect(buyHref()).toBe("/checkout?items=A%2CB%2CC");
  });

  it("[Cart selection: a shop select-all toggles that shop's lines] shop checkbox is indeterminate while only one of its lines is ticked", async () => {
    setup();
    const user = userEvent.setup();
    expect(shop("s1").indeterminate).toBe(false);
    await user.click(line("B"));
    expect(shop("s1").indeterminate).toBe(true);
    expect(shop("s1").checked).toBe(false);
    expect(shop("s2").indeterminate).toBe(false);
    await user.click(shop("s1"));
    expect(line("A").checked).toBe(true);
    expect(line("B").checked).toBe(true);
    expect(shop("s1").indeterminate).toBe(false);
  });

  it("[Cart selection: nothing ticked disables the buy action] nothing ticked disables buy", async () => {
    setup();
    const user = userEvent.setup();
    await user.click(shop("s1"));
    await user.click(shop("s2"));
    expect(buyLinks()).toHaveLength(0);
    const buttons = screen.getAllByRole("button", { name: "Mua hàng" });
    expect(buttons.length).toBeGreaterThan(0);
    for (const b of buttons) {
      expect(b).toBeDisabled();
      expect(b).not.toHaveAttribute("href");
    }
    expect(screen.getByText(/Tạm tính \(0 sản phẩm\)/)).toBeInTheDocument();
  });

  it("[Cart selection: the checkout kill switch keeps the buy action disabled] kill switch disables buy in the island", async () => {
    setup({ checkoutEnabled: false });
    expect(buyLinks()).toHaveLength(0);
    for (const b of screen.getAllByRole("button", { name: "Mua hàng" })) {
      expect(b).toBeDisabled();
    }
    expect(
      screen.getByText(/Thanh toán tạm thời không khả dụng/),
    ).toBeInTheDocument();
  });

  it("[Cart selection: a partial selection hides the cart discount and carries no voucher] partial selection hides discount and carries no voucher", async () => {
    setup({ discount: 10000, voucherCode: "V1" });
    const user = userEvent.setup();
    expect(screen.getByTestId("voucher-discount")).toHaveTextContent(
      "-₫10.000",
    );
    expect(
      screen.queryByText("Giảm giá được tính ở bước thanh toán"),
    ).not.toBeInTheDocument();
    expect(buyHref()).not.toContain("voucher");

    await user.click(line("C"));
    const href = buyHref() ?? "";
    expect(href).toBe("/checkout?items=A%2CB");
    const q = new URLSearchParams(href.split("?")[1]);
    for (const k of ["voucher", "sv", "pv"]) expect(q.has(k)).toBe(false);
    expect(screen.queryByTestId("voucher-discount")).not.toBeInTheDocument();
    expect(
      screen.getAllByText("Giảm giá được tính ở bước thanh toán").length,
    ).toBeGreaterThan(0);
    expect(screen.getByTestId("order-total")).toHaveTextContent("500.000");
  });

  it("[Cart selection: the back link from checkout restores the selection] initial selection comes from items param", () => {
    setup({ initialItems: "B,ghost" });
    expect(line("A").checked).toBe(false);
    expect(line("B").checked).toBe(true);
    expect(line("C").checked).toBe(false);
    expect(buyHref()).toBe("/checkout?items=B");
    expect(screen.getByText(/Tạm tính \(1 sản phẩm\)/)).toBeInTheDocument();
  });

  it("[Cart selection: an items parameter with no current line ticks everything] unmatched items param ticks every line", () => {
    setup({ initialItems: "ghost" });
    for (const id of ["A", "B", "C"]) expect(line(id).checked).toBe(true);
    expect(buyHref()).toBe("/checkout?items=A%2CB%2CC");
  });

  it("[Cart selection: the back link from checkout restores the selection] encoded items param is read", () => {
    setup({ initialItems: "A,C" });
    expect(line("B").checked).toBe(false);
    expect(buyHref()).toBe("/checkout?items=A%2CC");
  });

  it("[Cart selection: every line is ticked by default] rows render inside their shop card with one line checkbox each", () => {
    setup();
    const cards = screen.getAllByTestId("cart-shop-group");
    expect(
      within(cards[0] as HTMLElement).getAllByRole("listitem"),
    ).toHaveLength(2);
    expect(
      within(cards[0] as HTMLElement).getByTestId("cart-line-checkbox-A"),
    ).toBeInTheDocument();
    expect(
      within(cards[1] as HTMLElement).getByTestId("cart-shop-checkbox-s2"),
    ).toBeInTheDocument();
  });
});
