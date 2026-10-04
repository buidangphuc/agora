import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { PaymentMethod } from "@/generated/platform/payment/v1/payment_pb.js";
import { isCheckoutEnabled } from "@/lib/flags";
import type { ViewAddress } from "@/lib/gateway/addresses";
import { listAddresses } from "@/lib/gateway/addresses";
import type { ViewCartItem } from "@/lib/gateway/cart";
import { getCart } from "@/lib/gateway/cart";
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

function item(price: number, qty = 1): ViewCartItem {
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
  };
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
  render(await CheckoutPage({ searchParams }));
}

async function redirectOf(
  searchParams: Record<string, string | string[] | undefined> = {},
) {
  try {
    await CheckoutPage({ searchParams });
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
  it("redirects a signed-out visitor to /login", async () => {
    vi.mocked(getPrincipal).mockReturnValue(null);
    expect(await redirectOf()).toBe("/login");
  });

  it("redirects a signed-in buyer with an empty cart to /cart", async () => {
    setCart([]);
    expect(await redirectOf()).toBe("/cart");
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
    expect(
      screen.getByRole("link", { name: "Quay lại giỏ hàng" }),
    ).toHaveAttribute("href", "/cart");
    expect(screen.queryByTestId("order-summary")).toBeNull();
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
    const next = screen.getAllByRole("button", { name: "Tiếp tục" });
    for (const b of next) {
      expect(b).toBeDisabled();
      expect(b).toHaveAttribute("aria-disabled", "true");
    }
  });

  it("shipping step: Freeship tag at 500000 and above", async () => {
    setCart([item(250000, 2)]);
    await go({ step: "shipping", addr: "a1" });
    expect(screen.getAllByText("Freeship").length).toBeGreaterThanOrEqual(1);
    expect(screen.getByTestId("order-total")).toHaveTextContent("500.000");
  });

  it("shipping step: fee from the address city below the threshold", async () => {
    await go({ step: "shipping", addr: "a2" }); // Hồ Chí Minh -> 20000
    expect(screen.getAllByText("₫20.000").length).toBeGreaterThanOrEqual(1);
    expect(screen.getByTestId("order-total")).toHaveTextContent("220.000");
  });

  it("payment step: payment grid + voucher selector, back to shipping", async () => {
    await go({ step: "payment", addr: "a2", pay: "3" });
    expect(screen.getAllByRole("radio")).toHaveLength(4);
    expect(screen.getByRole("radio", { name: /VietQR/ })).toBeChecked();
    expect(
      screen.getByRole("button", { name: "Chọn hoặc nhập mã" }),
    ).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Quay lại" })).toHaveAttribute(
      "href",
      expect.stringContaining("step=shipping"),
    );
    expect(listVouchers).toHaveBeenCalledWith("s1");
  });

  it("confirm step: review, voucher discount in the summary and one Đặt hàng form", async () => {
    vi.mocked(previewVoucher).mockResolvedValue({
      valid: true,
      reason: "",
      discountAmount: 30000,
      voucherId: "v1",
    });
    await go({ step: "confirm", addr: "a2", voucher: "SAVE10" });
    expect(previewVoucher).toHaveBeenCalledWith("SAVE10", 200000, "s1");
    expect(screen.getByTestId("voucher-discount")).toHaveTextContent(
      "-₫30.000",
    );
    // 200000 - 30000 + 20000
    expect(screen.getByTestId("order-total")).toHaveTextContent("190.000");
    expect(
      screen.getAllByRole("button", { name: "Đặt hàng" }).length,
    ).toBeGreaterThanOrEqual(1);
    expect(screen.getByText("Áo thun")).toBeInTheDocument();
  });

  it("drops an invalid ?voucher= from the links and the summary", async () => {
    await go({ step: "shipping", addr: "a2", voucher: "BOGUS" });
    expect(screen.getByTestId("voucher-discount")).toHaveTextContent("-");
    const next = screen.getAllByRole("link", { name: "Tiếp tục" })[0];
    expect(next?.getAttribute("href")).not.toContain("voucher");
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
