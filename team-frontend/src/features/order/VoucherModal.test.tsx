import { render, screen, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { CheckoutVoucherScope } from "@/generated/platform/order/v1/order_pb.js";
import type { ViewCheckoutGroup, ViewVoucherQuote } from "@/lib/gateway/orders";
import { setupUser } from "@/test/user";

import { VoucherModal } from "./VoucherModal";
import { type CheckoutState } from "./checkoutParams";

const nav = vi.hoisted(() => ({ replace: vi.fn(), push: vi.fn() }));
const toast = vi.hoisted(() => ({
  success: vi.fn(),
  error: vi.fn(),
  info: vi.fn(),
  showToast: vi.fn(),
}));
vi.mock("@/components/ui/ToastProvider", () => ({ useToast: () => toast }));
vi.mock("next/navigation", () => ({
  useRouter: () => ({ replace: nav.replace, push: nav.push }),
  usePathname: () => "/checkout",
  useSearchParams: () => new URLSearchParams(),
}));
vi.mock("@/lib/analytics", () => ({ trackEcommerce: vi.fn() }));

function group(
  sellerId: string,
  extra: Partial<ViewCheckoutGroup> = {},
): ViewCheckoutGroup {
  return {
    sellerId,
    itemIds: [`ci-${sellerId}`],
    itemsSubtotal: 300000,
    shippingFee: 35000,
    isFreeShipping: false,
    shopVoucherCode: "",
    shopDiscountAmount: 0,
    platformDiscountAmount: 0,
    totalAmount: 335000,
    estimatedDeliveryAt: null,
    ...extra,
  };
}

function quote(extra: Partial<ViewVoucherQuote>): ViewVoucherQuote {
  return {
    code: "X",
    scope: CheckoutVoucherScope.SHOP,
    sellerId: "",
    valid: true,
    reason: "",
    discountAmount: 0,
    ...extra,
  };
}

const baseState: CheckoutState = {
  step: "payment",
  addr: "a1",
  pay: 0,
  items: [],
  sv: {},
};

function setup(
  over: {
    state?: Partial<CheckoutState>;
    groups?: ViewCheckoutGroup[];
    quotes?: ViewVoucherQuote[];
  } = {},
) {
  return render(
    <VoucherModal
      state={{ ...baseState, ...over.state }}
      groups={over.groups ?? [group("s1"), group("s2")]}
      quotes={over.quotes ?? []}
    />,
  );
}

function lastUrl(): URL {
  const call = nav.replace.mock.calls.at(-1);
  expect(call).toBeDefined();
  return new URL(String(call?.[0]), "http://localhost");
}

beforeEach(() => vi.clearAllMocks());

describe("VoucherModal scoped slots", () => {
  it("Checkout coherence: shop and platform vouchers go to their own slots: one shop field per group and one platform field", () => {
    setup();
    for (const id of ["s1", "s2"]) {
      const slot = screen.getByTestId(`shop-voucher-${id}`);
      expect(within(slot).getAllByRole("textbox")).toHaveLength(1);
    }
    const platform = screen.getByTestId("platform-voucher");
    expect(within(platform).getAllByRole("textbox")).toHaveLength(1);
    expect(screen.getAllByRole("textbox")).toHaveLength(3);
  });

  it("Checkout coherence: shop and platform vouchers go to their own slots: a shop code is written as sv=<seller>:<CODE>", async () => {
    const user = setupUser();
    setup();
    const slot = screen.getByTestId("shop-voucher-s1");
    await user.type(within(slot).getByRole("textbox"), "SHOPA10");
    await user.click(within(slot).getByRole("button", { name: "Áp dụng" }));
    const url = lastUrl();
    expect(url.pathname).toBe("/checkout");
    expect(url.searchParams.getAll("sv")).toEqual(["s1:SHOPA10"]);
    expect(url.searchParams.get("pv")).toBeNull();
    expect(url.searchParams.get("step")).toBe("payment");
    expect(url.searchParams.get("addr")).toBe("a1");
  });

  it("Checkout coherence: shop and platform vouchers go to their own slots: a platform code is written as pv and leaves sv alone", async () => {
    const user = setupUser();
    setup({ state: { sv: { s1: "SHOPA10" } } });
    const slot = screen.getByTestId("platform-voucher");
    await user.type(within(slot).getByRole("textbox"), "PLAT5");
    await user.click(within(slot).getByRole("button", { name: "Áp dụng" }));
    const url = lastUrl();
    expect(url.searchParams.get("pv")).toBe("PLAT5");
    expect(url.searchParams.getAll("sv")).toEqual(["s1:SHOPA10"]);
  });

  it("Checkout coherence: shop and platform vouchers go to their own slots: a second shop code keeps the first and at most one per seller", async () => {
    const user = setupUser();
    setup({ state: { sv: { s1: "SHOPA10" } } });
    const b = screen.getByTestId("shop-voucher-s2");
    await user.type(within(b).getByRole("textbox"), "SHOPB5");
    await user.click(within(b).getByRole("button", { name: "Áp dụng" }));
    expect(lastUrl().searchParams.getAll("sv").sort()).toEqual([
      "s1:SHOPA10",
      "s2:SHOPB5",
    ]);

    nav.replace.mockClear();
    const a = screen.getByTestId("shop-voucher-s1");
    await user.type(within(a).getByRole("textbox"), "SHOPA20");
    await user.click(within(a).getByRole("button", { name: "Áp dụng" }));
    expect(lastUrl().searchParams.getAll("sv")).toEqual(["s1:SHOPA20"]);
  });

  it("Checkout coherence: shop and platform vouchers go to their own slots: the apply button is disabled for a blank code", () => {
    setup();
    const slot = screen.getByTestId("shop-voucher-s1");
    expect(
      within(slot).getByRole("button", { name: "Áp dụng" }),
    ).toBeDisabled();
  });

  it("Checkout coherence: shop and platform vouchers go to their own slots: removing an applied shop code drops only that sv entry", async () => {
    const user = setupUser();
    setup({
      state: { sv: { s1: "SHOPA10", s2: "SHOPB5" }, pv: "PLAT5" },
      quotes: [
        quote({ code: "SHOPA10", sellerId: "s1" }),
        quote({ code: "SHOPB5", sellerId: "s2" }),
        quote({ code: "PLAT5", scope: CheckoutVoucherScope.PLATFORM }),
      ],
    });
    await user.click(screen.getByRole("button", { name: "Bỏ mã SHOPA10" }));
    const url = lastUrl();
    expect(url.searchParams.getAll("sv")).toEqual(["s2:SHOPB5"]);
    expect(url.searchParams.get("pv")).toBe("PLAT5");
  });

  it("Checkout coherence: shop and platform vouchers go to their own slots: group A shows its shop discount and platform share, group B a platform share only", () => {
    setup({
      state: { sv: { s1: "SHOPA10" }, pv: "PLAT5" },
      groups: [
        group("s1", {
          shopVoucherCode: "SHOPA10",
          shopDiscountAmount: 30000,
          platformDiscountAmount: 2500,
          totalAmount: 302500,
        }),
        group("s2", { platformDiscountAmount: 2500, totalAmount: 332500 }),
      ],
      quotes: [
        quote({ code: "SHOPA10", sellerId: "s1", discountAmount: 30000 }),
        quote({
          code: "PLAT5",
          scope: CheckoutVoucherScope.PLATFORM,
          discountAmount: 5000,
        }),
      ],
    });
    expect(screen.getByTestId("shop-discount-s1")).toHaveTextContent("30.000");
    expect(screen.getByTestId("platform-share-s1")).toHaveTextContent("2.500");
    expect(screen.getByTestId("shop-discount-s2").textContent?.trim()).toBe(
      "-",
    );
    expect(screen.getByTestId("platform-share-s2")).toHaveTextContent("2.500");
  });
});

describe("VoucherModal reasons", () => {
  it("Checkout coherence: an invalid voucher shows its reason and no discount: the reason appears in group B's slot only and its discount is -", () => {
    setup({
      state: { sv: { s2: "SHOPA10" } },
      groups: [group("s1"), group("s2")],
      quotes: [
        quote({
          code: "SHOPA10",
          sellerId: "s2",
          valid: false,
          reason: "voucher belongs to another shop",
        }),
      ],
    });
    const b = screen.getByTestId("shop-voucher-s2");
    expect(
      within(b).getByText("voucher belongs to another shop"),
    ).toBeInTheDocument();
    expect(
      within(screen.getByTestId("shop-voucher-s1")).queryByText(
        "voucher belongs to another shop",
      ),
    ).toBeNull();
    expect(
      within(screen.getByTestId("platform-voucher")).queryByText(
        "voucher belongs to another shop",
      ),
    ).toBeNull();
    expect(screen.getByTestId("shop-discount-s2").textContent?.trim()).toBe(
      "-",
    );
  });

  it("Checkout coherence: an invalid voucher shows its reason and no discount: an invalid platform code shows its reason in the platform slot", () => {
    setup({
      state: { pv: "NOPE" },
      quotes: [
        quote({
          code: "NOPE",
          scope: CheckoutVoucherScope.PLATFORM,
          valid: false,
          reason: "voucher expired",
        }),
      ],
    });
    expect(
      within(screen.getByTestId("platform-voucher")).getByText(
        "voucher expired",
      ),
    ).toBeInTheDocument();
    expect(screen.getByTestId("platform-share-s1").textContent?.trim()).toBe(
      "-",
    );
  });

  it("Checkout coherence: an invalid voucher shows its reason and no discount: a valid quote shows no reason text", () => {
    setup({
      state: { sv: { s1: "SHOPA10" } },
      groups: [
        group("s1", { shopDiscountAmount: 30000, shopVoucherCode: "SHOPA10" }),
      ],
      quotes: [
        quote({ code: "SHOPA10", sellerId: "s1", discountAmount: 30000 }),
      ],
    });
    expect(
      within(screen.getByTestId("shop-voucher-s1")).queryByRole("alert"),
    ).toBeNull();
  });
});
