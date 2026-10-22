import { Code, ConnectError } from "@connectrpc/connect";
import { render, screen, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { CheckoutVoucherScope } from "@/generated/platform/order/v1/order_pb.js";
import { PaymentMethod } from "@/generated/platform/payment/v1/payment_pb.js";
import { isCheckoutEnabled } from "@/lib/flags";
import type { ViewAddress } from "@/lib/gateway/addresses";
import { listAddresses } from "@/lib/gateway/addresses";
import type { ViewCartItem } from "@/lib/gateway/cart";
import { getCart } from "@/lib/gateway/cart";
import type {
  ViewCheckoutGroup,
  ViewCheckoutPreview,
} from "@/lib/gateway/orders";
import { calculateShippingFee, previewCheckout } from "@/lib/gateway/orders";
import { listVouchers, previewVoucher } from "@/lib/gateway/promotion";
import { getPrincipal } from "@/lib/gateway/session";

import CheckoutPage from "./page";

class RedirectSignal extends Error {}
const nav = vi.hoisted(() => ({
  redirect: vi.fn((url: string) => {
    throw new RedirectSignal(url);
  }),
}));

vi.mock("next/navigation", () => ({
  redirect: nav.redirect,
  notFound: vi.fn(),
  useRouter: () => ({ replace: vi.fn(), push: vi.fn() }),
  usePathname: () => "/checkout",
  useSearchParams: () => new URLSearchParams(),
}));
vi.mock("@/lib/flags", () => ({ isCheckoutEnabled: vi.fn() }));
vi.mock("@/lib/gateway/session", () => ({ getPrincipal: vi.fn() }));
vi.mock("@/lib/gateway/addresses", () => ({ listAddresses: vi.fn() }));
vi.mock("@/lib/gateway/cart", () => ({ getCart: vi.fn() }));
vi.mock("@/lib/gateway/orders", () => ({
  calculateShippingFee: vi.fn(),
  previewCheckout: vi.fn(),
}));
vi.mock("@/lib/gateway/promotion", () => ({
  listVouchers: vi.fn(),
  previewVoucher: vi.fn(),
}));
vi.mock("@/features/order/actions", () => ({ checkoutAction: vi.fn() }));
vi.mock("@/features/voucher/actions", () => ({
  previewVoucherAction: vi.fn(),
}));
vi.mock("@/features/address/AddressModal", () => ({
  AddressModal: () => null,
}));
vi.mock("@/lib/analytics", () => ({ trackEcommerce: vi.fn() }));

function addr(id: string, city: string, isDefault = false): ViewAddress {
  return {
    id,
    userId: "u1",
    recipientName: `Người ${id}`,
    phone: "0900000000",
    street: "1 Đường A",
    ward: "",
    district: "",
    city,
    isDefault,
  };
}

function item(
  price: number,
  qty = 1,
  extra: Partial<ViewCartItem> = {},
): ViewCartItem {
  return {
    id: "ci1",
    listingId: "l1",
    variantId: "",
    quantity: qty,
    unitPrice: price,
    title: "Áo thun",
    variantName: "",
    imageUrl: "",
    sellerId: "s1",
    sellerDisplayName: "",
    ...extra,
  };
}

function group(
  sellerId: string,
  extra: Partial<ViewCheckoutGroup> = {},
): ViewCheckoutGroup {
  return {
    sellerId,
    itemIds: ["ci1"],
    itemsSubtotal: 200000,
    shippingFee: 20000,
    isFreeShipping: false,
    shopVoucherCode: "",
    shopDiscountAmount: 0,
    platformDiscountAmount: 0,
    totalAmount: 220000,
    estimatedDeliveryAt: null,
    ...extra,
  };
}

function preview(
  extra: Partial<ViewCheckoutPreview> = {},
): ViewCheckoutPreview {
  return {
    groups: [group("s1")],
    vouchers: [],
    itemsSubtotal: 200000,
    shippingFee: 20000,
    discountAmount: 0,
    totalAmount: 220000,
    shippingQuoted: true,
    shippingAddress: null,
    ...extra,
  };
}

function setPreview(p: ViewCheckoutPreview) {
  vi.mocked(previewCheckout).mockResolvedValue(p);
}

function setCart(items: ViewCartItem[]) {
  vi.mocked(getCart).mockResolvedValue({
    userId: "u1",
    items,
    subtotal: items.reduce((a, i) => a + i.unitPrice * i.quantity, 0),
    totalItems: items.length,
  });
}

async function go(
  searchParams: Record<string, string | string[] | undefined> = {},
) {
  render(await CheckoutPage({ searchParams: Promise.resolve(searchParams) }));
}

async function redirectOf(
  searchParams: Record<string, string | string[] | undefined> = {},
) {
  try {
    await CheckoutPage({ searchParams: Promise.resolve(searchParams) });
  } catch (e) {
    if (e instanceof RedirectSignal) return e.message;
    throw e;
  }
  return null;
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(getPrincipal).mockReturnValue({
    id: "u1",
    name: "U",
    scopes: [],
  } as never);
  vi.mocked(isCheckoutEnabled).mockResolvedValue(true);
  vi.mocked(listAddresses).mockResolvedValue([
    addr("a1", "Đà Nẵng"),
    addr("a2", "Hồ Chí Minh", true),
  ]);
  setPreview(preview());
  vi.mocked(listVouchers).mockResolvedValue([]);
  vi.mocked(previewVoucher).mockResolvedValue({
    valid: false,
    reason: "",
    discountAmount: 0,
    voucherId: "",
  });
  setCart([item(100000, 2)]);
});

describe("/checkout page redirects and gates", () => {
  it("Checkout coherence: an anonymous visitor cannot reach checkout or pay pages: guest is redirected to login without any call", async () => {
    vi.mocked(getPrincipal).mockReturnValue(null);
    const url = await redirectOf({ step: "confirm", addr: "a2" });
    expect(url).toMatch(/^\/login/);
    expect(previewCheckout).not.toHaveBeenCalled();
    expect(getCart).not.toHaveBeenCalled();
    expect(listAddresses).not.toHaveBeenCalled();
    const { checkoutAction } = await import("@/features/order/actions");
    expect(checkoutAction).not.toHaveBeenCalled();
  });

  it("Checkout coherence: an empty preview selection returns to the cart: cart is empty redirects to the cart", async () => {
    vi.mocked(previewCheckout).mockRejectedValue(
      new ConnectError("cart is empty", Code.FailedPrecondition),
    );
    expect(await redirectOf({ step: "shipping", addr: "a2" })).toBe("/cart");
  });

  it("Checkout coherence: an empty preview selection returns to the cart: other failed preconditions are not turned into a redirect", async () => {
    vi.mocked(previewCheckout).mockRejectedValue(
      new ConnectError("address not found", Code.FailedPrecondition),
    );
    await expect(
      CheckoutPage({
        searchParams: Promise.resolve({ step: "shipping", addr: "a2" }),
      }),
    ).rejects.toThrow();
    expect(nav.redirect).not.toHaveBeenCalledWith("/cart");
  });

  it("redirects a skip-ahead with no address to step=address", async () => {
    const url = await redirectOf({ step: "confirm" });
    expect(url).toMatch(/^\/checkout\?step=address&addr=a2/);
  });

  it("redirects a stale ?addr= to step=address with the default address", async () => {
    const url = await redirectOf({ step: "payment", addr: "deleted" });
    expect(url).toContain("step=address");
    expect(url).toContain("addr=a2");
  });

  it("renders a Result (info) instead of the wizard when the kill-switch is off", async () => {
    vi.mocked(isCheckoutEnabled).mockResolvedValue(false);
    await go();
    expect(
      screen.getByText("Thanh toán tạm thời không khả dụng"),
    ).toBeInTheDocument();
    expect(screen.queryByTestId("order-summary")).toBeNull();
    expect(previewCheckout).not.toHaveBeenCalled();
  });
});

describe("/checkout groups and totals from the preview", () => {
  const twoShops = () => {
    vi.mocked(getCart).mockResolvedValue({
      userId: "u1",
      items: [
        item(300000, 1, {
          id: "ci1",
          sellerId: "s1",
          sellerDisplayName: "Shop A",
          title: "Áo A",
        }),
        item(300000, 1, {
          id: "ci2",
          sellerId: "s2",
          sellerDisplayName: "Shop B",
          title: "Áo B",
        }),
      ],
      subtotal: 600000,
      totalItems: 2,
    });
  };

  it("Checkout coherence: two shops show two groups with their own shipping: two shops show two groups with their own shipping", async () => {
    twoShops();
    setPreview(
      preview({
        groups: [
          group("s1", {
            itemIds: ["ci1"],
            itemsSubtotal: 300000,
            shippingFee: 35000,
            totalAmount: 335000,
          }),
          group("s2", {
            itemIds: ["ci2"],
            itemsSubtotal: 300000,
            shippingFee: 35000,
            totalAmount: 335000,
          }),
        ],
        itemsSubtotal: 600000,
        shippingFee: 70000,
        totalAmount: 670000,
      }),
    );
    await go({ step: "shipping", addr: "a1" });
    const a = screen.getByTestId("checkout-group-s1");
    const b = screen.getByTestId("checkout-group-s2");
    for (const g of [a, b]) {
      expect(g).toHaveTextContent("300.000");
      expect(g).toHaveTextContent("35.000");
      expect(within(g).queryByText("Freeship")).toBeNull();
    }
    expect(screen.getAllByTestId(/^checkout-group-/)).toHaveLength(2);
    const summary = screen.getByTestId("order-summary");
    expect(within(summary).getByText("₫70.000")).toBeInTheDocument();
    expect(within(summary).getByTestId("order-total")).toHaveTextContent(
      "670.000",
    );
  });

  it("Checkout coherence: free shipping is decided per group: free shipping is decided per group", async () => {
    twoShops();
    setPreview(
      preview({
        groups: [
          group("s1", {
            itemIds: ["ci1"],
            itemsSubtotal: 500000,
            shippingFee: 0,
            isFreeShipping: true,
            totalAmount: 500000,
          }),
          group("s2", {
            itemIds: ["ci2"],
            itemsSubtotal: 300000,
            shippingFee: 35000,
            totalAmount: 335000,
          }),
        ],
        itemsSubtotal: 800000,
        shippingFee: 35000,
        totalAmount: 835000,
      }),
    );
    await go({ step: "shipping", addr: "a1" });
    const a = screen.getByTestId("checkout-group-s1");
    const b = screen.getByTestId("checkout-group-s2");
    expect(within(a).getByText("Freeship")).toBeInTheDocument();
    expect(b).toHaveTextContent("35.000");
    expect(within(b).queryByText("Freeship")).toBeNull();
    const summary = screen.getByTestId("order-summary");
    expect(within(summary).getByText("₫35.000")).toBeInTheDocument();
    expect(within(summary).queryByText("Freeship")).toBeNull();
  });

  it.each(["address", "shipping", "payment", "confirm"])(
    "Checkout coherence: the frontend adds nothing up: summary shows the response totals unchanged at step %s",
    async (step) => {
      twoShops();
      setPreview(
        preview({
          groups: [
            group("s1", {
              itemIds: ["ci1"],
              itemsSubtotal: 300000,
              shippingFee: 35000,
              totalAmount: 335000,
            }),
            group("s2", {
              itemIds: ["ci2"],
              itemsSubtotal: 300000,
              shippingFee: 35000,
              totalAmount: 335000,
            }),
          ],
          itemsSubtotal: 612345,
          shippingFee: 41000,
          discountAmount: 7000,
          totalAmount: 999000,
        }),
      );
      await go({ step, addr: "a1" });
      const summary = screen.getByTestId("order-summary");
      expect(within(summary).getByTestId("order-total")).toHaveTextContent(
        "999.000",
      );
      expect(within(summary).getByText("₫612.345")).toBeInTheDocument();
      expect(within(summary).getByText("₫41.000")).toBeInTheDocument();
      expect(within(summary).getByTestId("voucher-discount")).toHaveTextContent(
        "-₫7.000",
      );
    },
  );

  it("Checkout coherence: a failed preview shows no total: a failed preview raises and shows no total", async () => {
    vi.mocked(previewCheckout).mockRejectedValue(
      new ConnectError("down", Code.Unavailable),
    );
    await expect(
      CheckoutPage({
        searchParams: Promise.resolve({ step: "shipping", addr: "a2" }),
      }),
    ).rejects.toThrow();
    expect(nav.redirect).not.toHaveBeenCalled();
    expect(screen.queryByTestId("order-total")).toBeNull();
  });

  it("Checkout coherence: each checkout group shows its own date: each group shows its own delivery date", async () => {
    twoShops();
    setPreview(
      preview({
        groups: [
          group("s1", {
            itemIds: ["ci1"],
            estimatedDeliveryAt: "2026-10-18T10:00:00+07:00",
          }),
          group("s2", {
            itemIds: ["ci2"],
            estimatedDeliveryAt: "2026-10-20T10:00:00+07:00",
          }),
        ],
        shippingQuoted: true,
      }),
    );
    await go({ step: "shipping", addr: "a1" });
    expect(screen.getByTestId("checkout-group-s1")).toHaveTextContent(
      "Nhận hàng dự kiến 18/10",
    );
    expect(screen.getByTestId("checkout-group-s2")).toHaveTextContent(
      "Nhận hàng dự kiến 20/10",
    );
  });

  it("Checkout coherence: each checkout group shows its own date: no date when shipping is not quoted", async () => {
    twoShops();
    setPreview(
      preview({
        groups: [
          group("s1", {
            itemIds: ["ci1"],
            estimatedDeliveryAt: "2026-10-18T10:00:00+07:00",
          }),
          group("s2", {
            itemIds: ["ci2"],
            estimatedDeliveryAt: "2026-10-20T10:00:00+07:00",
          }),
        ],
        shippingQuoted: false,
      }),
    );
    await go({ step: "shipping", addr: "a1" });
    expect(screen.queryByText(/Nhận hàng dự kiến/)).toBeNull();
  });

  it("Checkout coherence: the frontend adds nothing up: confirm step shows the preview address and the groups", async () => {
    twoShops();
    setPreview(
      preview({
        groups: [
          group("s1", { itemIds: ["ci1"] }),
          group("s2", { itemIds: ["ci2"] }),
        ],
        shippingAddress: {
          id: "a1",
          recipientName: "Người Preview",
          phone: "0911111111",
          street: "99 Phố Preview",
          ward: "",
          district: "",
          city: "Đà Nẵng",
        },
      }),
    );
    await go({ step: "confirm", addr: "a1" });
    expect(screen.getByText(/Người Preview/)).toBeInTheDocument();
    expect(screen.getByText(/99 Phố Preview/)).toBeInTheDocument();
    expect(screen.getAllByTestId(/^checkout-group-/)).toHaveLength(2);
    expect(
      screen.getAllByRole("button", { name: "Đặt hàng" }).length,
    ).toBeGreaterThanOrEqual(1);
  });
});

describe("/checkout items selection", () => {
  const threeLines = () =>
    vi.mocked(getCart).mockResolvedValue({
      userId: "u1",
      items: [
        item(100000, 1, { id: "ci1", title: "Sản phẩm A" }),
        item(100000, 1, { id: "ci2", title: "Sản phẩm B" }),
        item(100000, 1, { id: "ci3", title: "Sản phẩm C", sellerId: "s2" }),
      ],
      subtotal: 300000,
      totalItems: 3,
    });

  it("Checkout coherence: only the selected lines are quoted and listed: only the selected lines are quoted and listed", async () => {
    threeLines();
    setPreview(preview({ groups: [group("s1", { itemIds: ["ci2"] })] }));
    await go({ step: "confirm", addr: "a2", items: "ci2" });
    expect(previewCheckout).toHaveBeenCalledWith(
      expect.objectContaining({ itemIds: ["ci2"] }),
    );
    expect(screen.queryAllByText("Sản phẩm B").length).toBeGreaterThanOrEqual(
      1,
    );
    expect(screen.queryAllByText("Sản phẩm A")).toHaveLength(0);
    expect(screen.queryAllByText("Sản phẩm C")).toHaveLength(0);
  });

  it("Checkout coherence: no items param quotes the whole cart: no items param quotes the whole cart", async () => {
    threeLines();
    setPreview(
      preview({
        groups: [
          group("s1", { itemIds: ["ci1", "ci2"] }),
          group("s2", { itemIds: ["ci3"] }),
        ],
      }),
    );
    await go({ step: "confirm", addr: "a2" });
    expect(previewCheckout).toHaveBeenCalledWith(
      expect.objectContaining({ itemIds: [] }),
    );
    for (const t of ["Sản phẩm A", "Sản phẩm B", "Sản phẩm C"])
      expect(screen.queryAllByText(t).length).toBeGreaterThanOrEqual(1);
  });

  it("Checkout coherence: only the selected lines are quoted and listed: the address is sent to the preview", async () => {
    await go({ step: "shipping", addr: "a1" });
    expect(previewCheckout).toHaveBeenCalledWith(
      expect.objectContaining({ addressId: "a1" }),
    );
  });
});

describe("/checkout steps", () => {
  it("address step: default address selected, Tiếp tục carries addr into step=shipping", async () => {
    await go();
    expect(screen.getByText("Người a2")).toBeInTheDocument();
    expect(
      screen.getAllByRole("link", { name: "Tiếp tục" })[0],
    ).toHaveAttribute(
      "href",
      `/checkout?step=shipping&addr=a2&pay=${PaymentMethod.COD}`,
    );
  });

  it("no addresses: Empty state and a disabled Tiếp tục", async () => {
    vi.mocked(listAddresses).mockResolvedValue([]);
    await go();
    expect(
      screen.getByText("Bạn chưa có địa chỉ nhận hàng."),
    ).toBeInTheDocument();
    for (const b of screen.getAllByRole("button", { name: "Tiếp tục" })) {
      expect(b).toBeDisabled();
    }
  });

  it("mounts exactly one begin_checkout beacon-driven event for the page", async () => {
    const { trackEcommerce } = await import("@/lib/analytics");
    await go({ step: "shipping", addr: "a2" });
    expect(trackEcommerce).toHaveBeenCalledTimes(1);
    expect(trackEcommerce).toHaveBeenCalledWith(
      "begin_checkout",
      expect.objectContaining({ currency: "VND", value: 200000 }),
    );
  });
});

describe("/checkout keeps no local shipping, voucher or total rule", () => {
  it.each(["address", "shipping", "payment", "confirm"])(
    "Checkout coherence: checkout calls only the preview for figures: figures come from the preview only at step %s",
    async (step) => {
      await go({ step, addr: "a2", voucher: "OLD", pv: "PLAT5" });
      expect(previewCheckout).toHaveBeenCalled();
      expect(calculateShippingFee).not.toHaveBeenCalled();
      expect(previewVoucher).not.toHaveBeenCalled();
    },
  );
});

describe("/checkout scoped vouchers", () => {
  beforeEach(() => {
    vi.mocked(getCart).mockResolvedValue({
      userId: "u1",
      items: [
        item(300000, 1, { id: "ci1", sellerId: "s1", title: "Áo A" }),
        item(300000, 1, { id: "ci2", sellerId: "s2", title: "Áo B" }),
      ],
      subtotal: 600000,
      totalItems: 2,
    });
  });

  it("Checkout coherence: shop and platform vouchers go to their own slots: vouchers are sent in their own slots", async () => {
    setPreview(
      preview({
        groups: [
          group("s1", {
            itemIds: ["ci1"],
            itemsSubtotal: 300000,
            shopVoucherCode: "SHOPA10",
            shopDiscountAmount: 30000,
            platformDiscountAmount: 2500,
            totalAmount: 307500,
          }),
          group("s2", {
            itemIds: ["ci2"],
            itemsSubtotal: 300000,
            platformDiscountAmount: 2500,
            totalAmount: 332500,
          }),
        ],
        vouchers: [
          {
            code: "SHOPA10",
            scope: CheckoutVoucherScope.SHOP,
            sellerId: "s1",
            valid: true,
            reason: "",
            discountAmount: 30000,
          },
          {
            code: "PLAT5",
            scope: CheckoutVoucherScope.PLATFORM,
            sellerId: "",
            valid: true,
            reason: "",
            discountAmount: 5000,
          },
        ],
        itemsSubtotal: 600000,
        shippingFee: 70000,
        discountAmount: 35000,
        totalAmount: 635000,
      }),
    );
    await go({ step: "payment", addr: "a1", sv: "s1:SHOPA10", pv: "PLAT5" });
    expect(previewCheckout).toHaveBeenCalledWith(
      expect.objectContaining({
        shopVouchers: [{ sellerId: "s1", code: "SHOPA10" }],
        platformVoucherCode: "PLAT5",
      }),
    );
    expect(screen.getByTestId("shop-voucher-s1")).toBeInTheDocument();
    expect(screen.getByTestId("shop-voucher-s2")).toBeInTheDocument();
    expect(screen.getByTestId("platform-voucher")).toBeInTheDocument();
    expect(screen.getByTestId("shop-discount-s1")).toHaveTextContent("30.000");
    expect(screen.getByTestId("platform-share-s1")).toHaveTextContent("2.500");
    expect(screen.getByTestId("shop-discount-s2").textContent?.trim()).toBe(
      "-",
    );
    expect(screen.getByTestId("platform-share-s2")).toHaveTextContent("2.500");
  });

  it("Checkout coherence: shop and platform vouchers go to their own slots: available vouchers are listed per group seller", async () => {
    await go({ step: "payment", addr: "a1" });
    expect(listVouchers).toHaveBeenCalledWith("s1");
    expect(listVouchers).toHaveBeenCalledWith("s2");
  });

  it("Checkout coherence: an invalid voucher shows its reason and no discount: invalid voucher shows its reason", async () => {
    setPreview(
      preview({
        groups: [
          group("s1", {
            itemIds: ["ci1"],
            itemsSubtotal: 300000,
            shippingFee: 35000,
            totalAmount: 335000,
          }),
          group("s2", {
            itemIds: ["ci2"],
            itemsSubtotal: 300000,
            shippingFee: 35000,
            totalAmount: 335000,
          }),
        ],
        vouchers: [
          {
            code: "SHOPA10",
            scope: CheckoutVoucherScope.SHOP,
            sellerId: "s2",
            valid: false,
            reason: "voucher belongs to another shop",
            discountAmount: 0,
          },
        ],
        itemsSubtotal: 600000,
        shippingFee: 70000,
        discountAmount: 0,
        totalAmount: 670000,
      }),
    );
    await go({ step: "payment", addr: "a1", sv: "s2:SHOPA10" });
    const b = screen.getByTestId("shop-voucher-s2");
    expect(
      within(b).getByText("voucher belongs to another shop"),
    ).toBeInTheDocument();
    expect(
      within(screen.getByTestId("shop-voucher-s1")).queryByText(
        "voucher belongs to another shop",
      ),
    ).toBeNull();
    expect(screen.getByTestId("shop-discount-s2").textContent?.trim()).toBe(
      "-",
    );
    expect(
      within(screen.getByTestId("order-summary")).getByTestId("order-total"),
    ).toHaveTextContent("670.000");
  });
});
