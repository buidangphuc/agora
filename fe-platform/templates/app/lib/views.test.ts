import { OrderStatus, PaymentMethod } from "@fe-platform/api-client";
import { describe, expect, it, vi } from "vitest";

vi.mock("next/navigation", () => ({
  notFound: () => undefined,
  redirect: () => undefined,
}));
vi.mock("next/headers", () => ({ cookies: vi.fn() }));

import { toCartView } from "./cart";
import { toAddressView, toQuoteView } from "./checkout";
import { orderStatusLabel } from "./order-status";
import { toOrderView } from "./orders";
import { parsePaymentMethod, paymentOptions } from "./payment-methods";

describe("toCartView", () => {
  it("maps lines and formats money without summing", () => {
    const v = toCartView({
      items: [
        {
          id: "i1",
          listingId: "l1",
          title: "Lamp",
          variantName: "Blue",
          quantity: 2,
          unitPrice: 90000n,
          imageUrl: "",
        },
      ],
      subtotal: 180000n,
    } as never);
    expect(v.empty).toBe(false);
    expect(v.lines[0]).toMatchObject({
      id: "i1",
      quantity: 2,
      variantName: "Blue",
    });
    expect(v.lines[0]?.unitPrice).toContain("90,000");
    expect(v.subtotal).toContain("180,000");
  });

  it("reports an empty or missing cart", () => {
    expect(toCartView(undefined).empty).toBe(true);
    expect(toCartView({ items: [], subtotal: 0n } as never).empty).toBe(true);
  });
});

describe("checkout views", () => {
  it("joins the address and keeps the default flag", () => {
    const v = toAddressView({
      id: "a1",
      recipientName: "An",
      phone: "0900",
      street: "1 Main",
      ward: "",
      district: "D1",
      city: "HCM",
      isDefault: true,
    } as never);
    expect(v).toEqual({
      id: "a1",
      recipient: "An",
      phone: "0900",
      line: "1 Main, D1, HCM",
      isDefault: true,
    });
  });

  it("maps a quote", () => {
    const v = toQuoteView({
      groups: [
        {
          sellerId: "s",
          itemsSubtotal: 100n,
          shippingFee: 20n,
          isFreeShipping: false,
          totalAmount: 120n,
        },
      ],
      itemsSubtotal: 100n,
      shippingFee: 20n,
      shippingQuoted: true,
      discountAmount: 0n,
      totalAmount: 120n,
    } as never);
    expect(v.groups).toHaveLength(1);
    expect(v.shippingQuoted).toBe(true);
    expect(v.total).toContain("120");
  });
});

describe("toOrderView", () => {
  it("maps status, items and address using the order currency", () => {
    const v = toOrderView({
      id: "o1",
      status: OrderStatus.PAID,
      totalAmount: 5000n,
      currency: "VND",
      shippingFee: 0n,
      trackingNumber: "",
      shippingAddress: {
        recipientName: "An",
        street: "1 Main",
        ward: "",
        district: "",
        city: "HCM",
      },
      items: [
        {
          title: "Lamp",
          variantName: "",
          quantity: 1,
          unitPrice: 5000n,
          imageUrl: "",
        },
      ],
      createdAt: { toDate: () => new Date("2026-10-30T00:00:00Z") },
    } as never);
    expect(v.status).toBe("Paid");
    expect(v.address).toBe("1 Main, HCM");
    expect(v.items[0]?.title).toBe("Lamp");
    expect(v.placedOn).toContain("2026");
  });
});

describe("maps", () => {
  it("labels every known status and falls back to the enum name", () => {
    expect(orderStatusLabel(OrderStatus.SHIPPED)).toBe("Shipped");
    expect(orderStatusLabel(OrderStatus.UNSPECIFIED)).toBe("UNSPECIFIED");
  });

  it("offers every payment method except unspecified and parses the choice", () => {
    const options = paymentOptions();
    expect(options.length).toBeGreaterThan(0);
    expect(options.map((o) => o.value)).not.toContain(
      String(PaymentMethod.UNSPECIFIED),
    );
    expect(parsePaymentMethod(String(PaymentMethod.COD))).toBe(
      PaymentMethod.COD,
    );
    expect(parsePaymentMethod("99")).toBe(PaymentMethod.UNSPECIFIED);
    expect(parsePaymentMethod(null)).toBe(PaymentMethod.UNSPECIFIED);
  });
});
