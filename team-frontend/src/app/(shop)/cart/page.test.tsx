import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { isCheckoutEnabled } from "@/lib/flags";
import type { ViewCart, ViewCartItem } from "@/lib/gateway/cart";
import { getCartWithShopNames } from "@/lib/gateway/cart";
import { listVouchers, previewVoucher } from "@/lib/gateway/promotion";
import { getPrincipal } from "@/lib/gateway/session";
import { redirect } from "next/navigation";

import CartPage from "./page";

vi.mock("@/lib/flags", () => ({ isCheckoutEnabled: vi.fn() }));
vi.mock("@/lib/gateway/cart", () => ({ getCartWithShopNames: vi.fn() }));
vi.mock("@/lib/gateway/promotion", () => ({
  listVouchers: vi.fn(),
  previewVoucher: vi.fn(),
}));
vi.mock("@/features/cart/actions", () => ({
  updateCartItemAction: vi.fn(),
  removeFromCartAction: vi.fn(),
  clearCartAction: vi.fn(),
}));
vi.mock("@/features/voucher/actions", () => ({
  previewVoucherAction: vi.fn(),
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

function item(id: string, sellerId: string, name: string): ViewCartItem {
  return {
    id,
    listingId: `l-${id}`,
    variantId: "",
    quantity: 1,
    unitPrice: 100000,
    title: `Sản phẩm ${id}`,
    variantName: "",
    imageUrl: "",
    sellerId,
    sellerDisplayName: name,
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

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(getPrincipal).mockReturnValue({ id: "u1" } as never);
  vi.mocked(isCheckoutEnabled).mockResolvedValue(true);
  vi.mocked(listVouchers).mockResolvedValue([]);
  vi.mocked(previewVoucher).mockResolvedValue({
    valid: false,
    reason: "",
    discountAmount: 0,
    voucherId: "",
  });
});

async function renderPage(searchParams = {}) {
  render(await CartPage({ searchParams: Promise.resolve(searchParams) }));
}

describe("/cart page guard", () => {
  it("[Cart page guard covers update and remove] cart page redirects with returnUrl", async () => {
    vi.mocked(getPrincipal).mockReturnValue(null);
    await expect(renderPage()).rejects.toThrow("NEXT_REDIRECT");
    expect(redirect).toHaveBeenCalledWith("/login?returnUrl=%2Fcart");
    expect(getCartWithShopNames).not.toHaveBeenCalled();
  });

  it("[Cart page guard covers update and remove] keeps the voucher query", async () => {
    vi.mocked(getPrincipal).mockReturnValue(null);
    await expect(renderPage({ voucher: "A1" })).rejects.toThrow(
      "NEXT_REDIRECT",
    );
    expect(redirect).toHaveBeenCalledWith(
      "/login?returnUrl=%2Fcart%3Fvoucher%3DA1",
    );
  });

  it("[Cart page guard covers update and remove] a signed-in shopper is not redirected", async () => {
    vi.mocked(getCartWithShopNames).mockResolvedValue(cartOf([]));
    await renderPage();
    expect(redirect).not.toHaveBeenCalled();
  });
});

describe("/cart page", () => {
  it("renders the Empty state with a continue-shopping link and no summary or buy button", async () => {
    vi.mocked(getCartWithShopNames).mockResolvedValue(cartOf([]));
    await renderPage();
    expect(screen.getByText("Giỏ hàng của bạn đang trống")).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: "Tiếp tục mua sắm" }),
    ).toHaveAttribute("href", "/");
    expect(screen.queryByTestId("order-summary")).not.toBeInTheDocument();
    expect(screen.queryByText("Mua hàng")).not.toBeInTheDocument();
  });

  it("groups two shops, shows real names and the summary subtotal", async () => {
    vi.mocked(getCartWithShopNames).mockResolvedValue(
      cartOf([item("a", "s1", "Cửa hàng Hoa Mai"), item("b", "s2", "")]),
    );
    await renderPage();
    expect(screen.getAllByTestId("cart-shop-group")).toHaveLength(2);
    expect(screen.getByText("Cửa hàng Hoa Mai")).toBeInTheDocument();
    expect(screen.getByText("Shop #s2")).toBeInTheDocument();
    expect(screen.getByTestId("order-total")).toHaveTextContent("200.000");
    expect(
      screen.getByRole("button", { name: "Xóa tất cả" }),
    ).toBeInTheDocument();
    expect(
      screen.getAllByRole("link", { name: "Mua hàng" })[0],
    ).toHaveAttribute("href", "/checkout?items=a%2Cb");
  });

  it("disables Mua hàng and explains with an Alert when the kill-switch is off", async () => {
    vi.mocked(isCheckoutEnabled).mockResolvedValue(false);
    vi.mocked(getCartWithShopNames).mockResolvedValue(
      cartOf([item("a", "s1", "Shop")]),
    );
    await renderPage();
    const buttons = screen.getAllByRole("button", { name: "Mua hàng" });
    for (const b of buttons) {
      expect(b).toBeDisabled();
      expect(b).toHaveAttribute("aria-disabled", "true");
    }
    expect(
      screen.getByText(/Thanh toán tạm thời không khả dụng/),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("link", { name: "Mua hàng" }),
    ).not.toBeInTheDocument();
  });

  it("applies a server-previewed voucher to the summary and keeps it out of the checkout link", async () => {
    vi.mocked(getCartWithShopNames).mockResolvedValue(
      cartOf([item("a", "s1", "Shop")]),
    );
    vi.mocked(previewVoucher).mockResolvedValue({
      valid: true,
      reason: "",
      discountAmount: 10000,
      voucherId: "v1",
    });
    await renderPage({ voucher: "SAVE10" });
    expect(previewVoucher).toHaveBeenCalledWith("SAVE10", 100000, "s1");
    expect(screen.getByTestId("voucher-discount")).toHaveTextContent(
      "-₫10.000",
    );
    expect(screen.getByTestId("order-total")).toHaveTextContent("90.000");
    expect(
      screen.getAllByRole("link", { name: "Mua hàng" })[0],
    ).toHaveAttribute("href", "/checkout?items=a");
  });

  it("ignores an invalid ?voucher= (no discount, selection-only checkout link)", async () => {
    vi.mocked(getCartWithShopNames).mockResolvedValue(
      cartOf([item("a", "s1", "Shop")]),
    );
    await renderPage({ voucher: "BOGUS" });
    expect(screen.getByTestId("voucher-discount")).toHaveTextContent("-");
    expect(
      screen.getAllByRole("link", { name: "Mua hàng" })[0],
    ).toHaveAttribute("href", "/checkout?items=a");
  });

  it("reserves bottom padding for the mobile bar and has the bar", async () => {
    vi.mocked(getCartWithShopNames).mockResolvedValue(
      cartOf([item("a", "s1", "Shop")]),
    );
    await renderPage();
    expect(screen.getByTestId("mobile-summary-bar")).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { level: 1 }).closest("section"),
    ).toHaveClass("pb-24");
  });

  it("[Cart selection: the checkout kill switch keeps the buy action disabled] kill switch disables buy even with a selection", async () => {
    vi.mocked(isCheckoutEnabled).mockResolvedValue(false);
    vi.mocked(getCartWithShopNames).mockResolvedValue(
      cartOf([item("a", "s1", "Shop"), item("b", "s1", "Shop")]),
    );
    await renderPage({ items: "a,b" });
    expect(screen.getByTestId("cart-line-checkbox-a")).toBeChecked();
    expect(
      screen.queryByRole("link", { name: "Mua hàng" }),
    ).not.toBeInTheDocument();
    for (const b of screen.getAllByRole("button", { name: "Mua hàng" })) {
      expect(b).toBeDisabled();
    }
    expect(
      screen.getByText(/Thanh toán tạm thời không khả dụng/),
    ).toBeInTheDocument();
  });

  it("[Cart selection: an anonymous visitor cannot reach the selectable cart] anonymous is redirected to login", async () => {
    vi.mocked(getPrincipal).mockReturnValue(null);
    await expect(renderPage({ items: "l1" })).rejects.toThrow("NEXT_REDIRECT");
    expect(redirect).toHaveBeenCalledWith(
      "/login?returnUrl=%2Fcart%3Fitems%3Dl1",
    );
    expect(getCartWithShopNames).not.toHaveBeenCalled();
    expect(screen.queryAllByRole("checkbox")).toHaveLength(0);
  });

  it("[Cart selection: the back link from checkout restores the selection] page seeds the island from ?items= and ignores unknown ids", async () => {
    vi.mocked(getCartWithShopNames).mockResolvedValue(
      cartOf([
        item("a", "s1", "Shop"),
        item("b", "s1", "Shop"),
        item("c", "s2", "Shop 2"),
      ]),
    );
    await renderPage({ items: "b,ghost" });
    expect(screen.getByTestId("cart-line-checkbox-a")).not.toBeChecked();
    expect(screen.getByTestId("cart-line-checkbox-b")).toBeChecked();
    expect(screen.getByTestId("cart-line-checkbox-c")).not.toBeChecked();
    expect(
      screen.getAllByRole("link", { name: "Mua hàng" })[0],
    ).toHaveAttribute("href", "/checkout?items=b");
  });

  it("[Cart selection: an items parameter with no current line ticks everything] page ticks every line when ?items= names no cart line", async () => {
    vi.mocked(getCartWithShopNames).mockResolvedValue(
      cartOf([item("a", "s1", "Shop"), item("b", "s2", "Shop 2")]),
    );
    await renderPage({ items: "ghost" });
    expect(screen.getByTestId("cart-line-checkbox-a")).toBeChecked();
    expect(screen.getByTestId("cart-line-checkbox-b")).toBeChecked();
  });

  it("[Cart selection: every line is ticked by default] page renders a row and a checkbox per line server-side", async () => {
    vi.mocked(getCartWithShopNames).mockResolvedValue(
      cartOf([
        item("a", "s1", "Shop"),
        item("b", "s1", "Shop"),
        item("c", "s2", "Shop 2"),
      ]),
    );
    await renderPage();
    expect(screen.getAllByTestId(/^cart-line-checkbox-/)).toHaveLength(3);
    expect(screen.getAllByTestId(/^cart-shop-checkbox-/)).toHaveLength(2);
  });
});
