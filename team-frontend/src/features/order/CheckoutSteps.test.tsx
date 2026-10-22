import { render, screen, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { CheckoutVoucherScope } from "@/generated/platform/order/v1/order_pb.js";
import type { ViewCheckoutGroup, ViewVoucherQuote } from "@/lib/gateway/orders";

import { PaymentStep } from "./CheckoutSteps";
import type { CheckoutState } from "./checkoutParams";

vi.mock("@/components/ui/ToastProvider", () => ({
  useToast: () => ({
    success: vi.fn(),
    error: vi.fn(),
    info: vi.fn(),
    showToast: vi.fn(),
  }),
}));
vi.mock("next/navigation", () => ({
  useRouter: () => ({ replace: vi.fn(), push: vi.fn() }),
  usePathname: () => "/checkout",
  useSearchParams: () => new URLSearchParams(),
}));
vi.mock("@/lib/analytics", () => ({ trackEcommerce: vi.fn() }));

const state: CheckoutState = {
  step: "payment",
  addr: "a1",
  pay: 0,
  items: [],
  sv: { s2: "SHOPA10" },
  pv: "PLAT5",
};

function group(sellerId: string): ViewCheckoutGroup {
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
  };
}

const quotes: ViewVoucherQuote[] = [
  {
    code: "SHOPA10",
    scope: CheckoutVoucherScope.SHOP,
    sellerId: "s2",
    valid: false,
    reason: "voucher belongs to another shop",
    discountAmount: 0,
  },
  {
    code: "PLAT5",
    scope: CheckoutVoucherScope.PLATFORM,
    sellerId: "",
    valid: false,
    reason: "voucher expired",
    discountAmount: 0,
  },
];

beforeEach(() => vi.clearAllMocks());

describe("PaymentStep scoped voucher entry", () => {
  it("Checkout coherence: shop and platform vouchers go to their own slots: the payment step has one field per group and one platform field", () => {
    render(
      <PaymentStep
        state={state}
        groups={[group("s1"), group("s2")]}
        quotes={quotes}
      />,
    );
    expect(screen.getByTestId("shop-voucher-s1")).toBeInTheDocument();
    expect(screen.getByTestId("shop-voucher-s2")).toBeInTheDocument();
    expect(screen.getByTestId("platform-voucher")).toBeInTheDocument();
  });

  it("Checkout coherence: an invalid voucher shows its reason and no discount: each reason sits next to its own field", () => {
    render(
      <PaymentStep
        state={state}
        groups={[group("s1"), group("s2")]}
        quotes={quotes}
      />,
    );
    expect(
      within(screen.getByTestId("shop-voucher-s2")).getByText(
        "voucher belongs to another shop",
      ),
    ).toBeInTheDocument();
    expect(
      within(screen.getByTestId("platform-voucher")).getByText(
        "voucher expired",
      ),
    ).toBeInTheDocument();
    expect(
      within(screen.getByTestId("shop-voucher-s1")).queryByText(
        "voucher belongs to another shop",
      ),
    ).toBeNull();
  });
});
