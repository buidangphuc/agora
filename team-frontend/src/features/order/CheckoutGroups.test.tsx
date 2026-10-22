import { render, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import type { ViewCartItem } from "@/lib/gateway/cart";
import type { ViewCheckoutGroup } from "@/lib/gateway/orders";

import { CheckoutGroups } from "./CheckoutGroups";

function group(
  sellerId: string,
  extra: Partial<ViewCheckoutGroup> = {},
): ViewCheckoutGroup {
  return {
    sellerId,
    itemIds: [],
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

function line(
  id: string,
  sellerId: string,
  title: string,
  shop: string,
): ViewCartItem {
  return {
    id,
    listingId: `l-${id}`,
    variantId: "",
    quantity: 1,
    unitPrice: 300000,
    title,
    variantName: "",
    imageUrl: "",
    sellerId,
    sellerDisplayName: shop,
  };
}

const items = [
  line("i1", "sA", "Áo A", "Shop A"),
  line("i2", "sB", "Áo B", "Shop B"),
];

describe("CheckoutGroups", () => {
  it("Checkout coherence: two shops show two groups with their own shipping: one group per quote with subtotal, fee and total", () => {
    render(
      <CheckoutGroups
        groups={[
          group("sA", { itemIds: ["i1"] }),
          group("sB", {
            itemIds: ["i2"],
            shippingFee: 20000,
            totalAmount: 320000,
          }),
        ]}
        items={items}
        shippingQuoted
      />,
    );
    const a = screen.getByTestId("checkout-group-sA");
    const b = screen.getByTestId("checkout-group-sB");
    expect(a).toHaveTextContent("Shop A");
    expect(a).toHaveTextContent("300.000");
    expect(a).toHaveTextContent("35.000");
    expect(a).toHaveTextContent("335.000");
    expect(b).toHaveTextContent("Shop B");
    expect(b).toHaveTextContent("20.000");
    expect(b).toHaveTextContent("320.000");
  });

  it("Checkout coherence: free shipping is decided per group: Freeship only on the group the server marked free", () => {
    render(
      <CheckoutGroups
        groups={[
          group("sA", {
            itemIds: ["i1"],
            isFreeShipping: true,
            shippingFee: 0,
            totalAmount: 300000,
          }),
          group("sB", { itemIds: ["i2"] }),
        ]}
        items={items}
        shippingQuoted
      />,
    );
    expect(
      within(screen.getByTestId("checkout-group-sA")).getByText("Freeship"),
    ).toBeInTheDocument();
    expect(
      within(screen.getByTestId("checkout-group-sB")).queryByText("Freeship"),
    ).toBeNull();
  });

  it("Checkout coherence: the frontend adds nothing up: shows the group total as given, not subtotal + shipping - discounts", () => {
    render(
      <CheckoutGroups
        groups={[
          group("sA", {
            itemIds: ["i1"],
            shopDiscountAmount: 10000,
            platformDiscountAmount: 5000,
            totalAmount: 123456,
          }),
        ]}
        items={items}
        shippingQuoted
      />,
    );
    const a = screen.getByTestId("checkout-group-sA");
    expect(a).toHaveTextContent("123.456");
    expect(a).toHaveTextContent("-₫10.000");
    expect(a).toHaveTextContent("-₫5.000");
    expect(a).not.toHaveTextContent("320.000");
  });

  it("Checkout coherence: each checkout group shows its own date: dd/mm per group in Ho Chi Minh time, none when not quoted or unset", () => {
    const groups = [
      group("sA", {
        itemIds: ["i1"],
        estimatedDeliveryAt: "2026-10-17T18:30:00Z",
      }),
      group("sB", { itemIds: ["i2"], estimatedDeliveryAt: null }),
    ];
    const { rerender } = render(
      <CheckoutGroups groups={groups} items={items} shippingQuoted />,
    );
    expect(screen.getByTestId("checkout-group-sA")).toHaveTextContent(
      "Nhận hàng dự kiến 18/10",
    );
    expect(
      within(screen.getByTestId("checkout-group-sB")).queryByText(
        /Nhận hàng dự kiến/,
      ),
    ).toBeNull();
    rerender(
      <CheckoutGroups groups={groups} items={items} shippingQuoted={false} />,
    );
    expect(screen.queryByText(/Nhận hàng dự kiến/)).toBeNull();
  });

  it("Checkout coherence: only the selected lines are quoted and listed: a group lists only the lines in its itemIds", () => {
    render(
      <CheckoutGroups
        groups={[group("sA", { itemIds: ["i1"] })]}
        items={items}
        shippingQuoted
      />,
    );
    const a = screen.getByTestId("checkout-group-sA");
    expect(within(a).getByText("Áo A")).toBeInTheDocument();
    expect(screen.queryByText("Áo B")).toBeNull();
  });
});
