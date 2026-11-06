import { render, screen, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { CheckoutVoucherScope } from "@/generated/platform/order/v1/order_pb.js";
import { trackEcommerce } from "@/lib/analytics";
import type { ViewCheckoutGroup, ViewVoucherQuote } from "@/lib/gateway/orders";
import type { ViewVoucher } from "@/lib/gateway/promotion";
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

describe("VoucherModal tracking", () => {
  it("apply_promotion still fires for valid and invalid codes: a valid then an invalid platform preview push valid true then false", async () => {
    const user = setupUser();
    const view = setup();
    const slot = () => screen.getByTestId("platform-voucher");
    await user.type(within(slot()).getByRole("textbox"), "SAVE10");
    await user.click(within(slot()).getByRole("button", { name: "Áp dụng" }));
    expect(trackEcommerce).not.toHaveBeenCalled();
    view.rerender(
      <VoucherModal
        state={{ ...baseState, pv: "SAVE10" }}
        groups={[group("s1"), group("s2")]}
        quotes={[
          quote({
            code: "SAVE10",
            scope: CheckoutVoucherScope.PLATFORM,
            discountAmount: 30000,
          }),
        ]}
      />,
    );
    await user.type(within(slot()).getByRole("textbox"), "NOPE");
    await user.click(within(slot()).getByRole("button", { name: "Áp dụng" }));
    view.rerender(
      <VoucherModal
        state={{ ...baseState, pv: "NOPE" }}
        groups={[group("s1"), group("s2")]}
        quotes={[
          quote({
            code: "NOPE",
            scope: CheckoutVoucherScope.PLATFORM,
            valid: false,
            reason: "voucher not found",
          }),
        ]}
      />,
    );
    expect(vi.mocked(trackEcommerce).mock.calls).toEqual([
      [
        "apply_promotion",
        { coupon: "SAVE10", value: 30000, properties: { valid: "true" } },
      ],
      [
        "apply_promotion",
        { coupon: "NOPE", value: 0, properties: { valid: "false" } },
      ],
    ]);
  });

  it("apply_promotion still fires for valid and invalid codes: a shop preview pushes once with its own quote", async () => {
    const user = setupUser();
    const view = setup();
    const slot = screen.getByTestId("shop-voucher-s1");
    await user.type(within(slot).getByRole("textbox"), "SHOPA10");
    await user.click(within(slot).getByRole("button", { name: "Áp dụng" }));
    const next = (
      <VoucherModal
        state={{ ...baseState, sv: { s1: "SHOPA10" } }}
        groups={[group("s1"), group("s2")]}
        quotes={[
          quote({ code: "SHOPA10", sellerId: "s1", discountAmount: 25000 }),
        ]}
      />
    );
    view.rerender(next);
    view.rerender(next);
    expect(vi.mocked(trackEcommerce).mock.calls).toEqual([
      [
        "apply_promotion",
        { coupon: "SHOPA10", value: 25000, properties: { valid: "true" } },
      ],
    ]);
  });

  it("apply_promotion still fires for valid and invalid codes: quotes already in the URL on load push nothing", () => {
    setup({
      state: { pv: "SAVE10" },
      quotes: [
        quote({
          code: "SAVE10",
          scope: CheckoutVoucherScope.PLATFORM,
          discountAmount: 30000,
        }),
      ],
    });
    expect(trackEcommerce).not.toHaveBeenCalled();
  });
});

describe("VoucherModal inactive reason", () => {
  it("V28a: a shop quote refused with reason inactive shows the Vietnamese text and no raw reason", () => {
    setup({
      state: { sv: { s1: "STOPPED" } },
      quotes: [
        quote({
          code: "STOPPED",
          sellerId: "s1",
          valid: false,
          reason: "inactive",
        }),
      ],
    });
    const slot = screen.getByTestId("shop-voucher-s1");
    expect(
      within(slot).getByText("Mã giảm giá đã ngừng áp dụng."),
    ).toBeInTheDocument();
    expect(screen.queryByText("inactive")).toBeNull();
  });

  it("V28a: a platform quote refused with reason inactive shows the Vietnamese text and no raw reason", () => {
    setup({
      state: { pv: "STOPPED" },
      quotes: [
        quote({
          code: "STOPPED",
          scope: CheckoutVoucherScope.PLATFORM,
          valid: false,
          reason: "inactive",
        }),
      ],
    });
    expect(
      within(screen.getByTestId("platform-voucher")).getByText(
        "Mã giảm giá đã ngừng áp dụng.",
      ),
    ).toBeInTheDocument();
    expect(screen.queryByText("inactive")).toBeNull();
  });

  it("V28b: a reason other than inactive keeps its text", () => {
    setup({
      state: { sv: { s1: "OLD" } },
      quotes: [
        quote({
          code: "OLD",
          sellerId: "s1",
          valid: false,
          reason: "voucher expired",
        }),
      ],
    });
    expect(
      within(screen.getByTestId("shop-voucher-s1")).getByText(
        "voucher expired",
      ),
    ).toBeInTheDocument();
    expect(screen.queryByText("Mã giảm giá đã ngừng áp dụng.")).toBeNull();
  });
});

function ownVoucher(
  code: string,
  over: Partial<ViewVoucher> = {},
): ViewVoucher {
  return {
    id: `id-${code}`,
    code,
    scope: 2,
    scopeText: "Toàn sàn",
    sellerId: "",
    discountType: 2,
    discountTypeText: "Giảm tiền",
    discountValue: 50000,
    minSpend: 200000,
    maxDiscount: 0,
    quota: 1,
    used: 0,
    startsAt: "",
    endsAt: "30/11/2026",
    kind: 2,
    ownerUserId: "u1",
    ...over,
  } as ViewVoucher;
}

function setupWithOwn(
  myVouchers: ViewVoucher[],
  over: {
    state?: Partial<CheckoutState>;
    quotes?: ViewVoucherQuote[];
    platformAvailable?: ViewVoucher[];
  } = {},
) {
  return render(
    <VoucherModal
      state={{ ...baseState, ...over.state }}
      groups={[group("s1")]}
      quotes={over.quotes ?? []}
      platformAvailable={over.platformAvailable}
      {...({ myVouchers } as object)}
    />,
  );
}

describe("VoucherModal own vouchers", () => {
  it("W6 lists the buyer's own voucher code in the platform slot only", () => {
    setupWithOwn([ownVoucher("WELCOME50")]);
    expect(
      within(screen.getByTestId("platform-voucher")).getByRole("button", {
        name: "WELCOME50",
      }),
    ).toBeInTheDocument();
    expect(
      within(screen.getByTestId("shop-voucher-s1")).queryByRole("button", {
        name: "WELCOME50",
      }),
    ).toBeNull();
  });

  it("W6 applying the own voucher writes it as pv in the checkout url", async () => {
    const user = setupUser();
    setupWithOwn([ownVoucher("WELCOME50")], {
      state: { sv: { s1: "SHOPA10" } },
    });
    await user.click(
      within(screen.getByTestId("platform-voucher")).getByRole("button", {
        name: "WELCOME50",
      }),
    );
    const url = lastUrl();
    expect(url.searchParams.get("pv")).toBe("WELCOME50");
    expect(url.searchParams.getAll("sv")).toEqual(["s1:SHOPA10"]);
    expect(url.searchParams.get("step")).toBe("payment");
  });

  it("W6 own vouchers come before platform vouchers in the slot", () => {
    setupWithOwn([ownVoucher("WELCOME50")], {
      platformAvailable: [ownVoucher("PLAT5", { kind: 1, ownerUserId: "" })],
    });
    const names = within(screen.getByTestId("platform-voucher"))
      .getAllByRole("button")
      .map((b) => b.textContent);
    expect(names.indexOf("WELCOME50")).toBeGreaterThanOrEqual(0);
    expect(names.indexOf("WELCOME50")).toBeLessThan(names.indexOf("PLAT5"));
  });

  it("W6 a code present in both lists is offered once", () => {
    setupWithOwn([ownVoucher("WELCOME50")], {
      platformAvailable: [ownVoucher("WELCOME50"), ownVoucher("PLAT5")],
    });
    const slot = within(screen.getByTestId("platform-voucher"));
    expect(slot.getAllByRole("button", { name: "WELCOME50" })).toHaveLength(1);
    expect(slot.getByRole("button", { name: "PLAT5" })).toBeInTheDocument();
  });

  it("W7 Picker survives a read failure", () => {
    setupWithOwn([], {
      platformAvailable: [ownVoucher("PLAT5", { kind: 1, ownerUserId: "" })],
    });
    expect(
      within(screen.getByTestId("platform-voucher")).getByRole("button", {
        name: "PLAT5",
      }),
    ).toBeInTheDocument();
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("W8 an own voucher refused by the quote shows the quote reason in the platform slot", () => {
    setupWithOwn([ownVoucher("WELCOME50")], {
      state: { pv: "WELCOME50" },
      quotes: [
        quote({
          code: "WELCOME50",
          scope: CheckoutVoucherScope.PLATFORM,
          valid: false,
          reason: "below minimum spend",
        }),
      ],
    });
    const slot = within(screen.getByTestId("platform-voucher"));
    expect(slot.getByText("below minimum spend")).toBeInTheDocument();
    expect(slot.getByRole("button", { name: "WELCOME50" })).toBeInTheDocument();
    expect(screen.getByTestId("platform-share-s1").textContent?.trim()).toBe(
      "-",
    );
  });
});
