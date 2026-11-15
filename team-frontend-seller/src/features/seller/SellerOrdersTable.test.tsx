import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { OrderStatus } from "@/generated/platform/order/v1/order_pb.js";
import { PaymentMethod } from "@/generated/platform/payment/v1/payment_pb.js";
import type { ViewOrder } from "@/lib/gateway/orders";
import { BulkShipSelectionProvider } from "./BulkShipSelection";
import { SellerOrdersTable } from "./SellerOrdersTable";

vi.mock("@/features/order/actions", () => ({
  updateOrderStatusAction: vi.fn(),
  bulkShipOrdersAction: vi.fn(),
}));
vi.mock("@/components/ui/ToastProvider", () => ({
  useToast: () => ({ success: vi.fn(), error: vi.fn() }),
}));

function order(
  id: string,
  status: OrderStatus,
  paymentMethod: PaymentMethod,
): ViewOrder {
  return {
    id,
    buyerId: "b",
    sellerId: "me",
    status,
    statusText: "x",
    totalAmount: 1000,
    itemsSubtotal: 1000,
    shippingFee: 0,
    paymentMethod,
    paymentMethodText: "x",
    currency: "VND",
    discountAmount: 0,
    voucherCode: "",
    checkoutId: "",
    platformVoucherCode: "",
    platformDiscountAmount: 0,
    recipientName: `Khach ${id}`,
    phone: "0900000000",
    addressFull: "addr",
    trackingNumber: "",
    items: [],
    createdAt: "01/10/2026",
    paidAt: "",
    paymentDueAt: "",
    shipBy: "",
    shippedAt: "",
    completedAt: "",
    returnWindowEndsAt: "",
  } as unknown as ViewOrder;
}

describe("SellerOrdersTable selection", () => {
  it("Bulk ship: only shippable rows can be selected: checkbox only on shippable rows", () => {
    const orders = [
      order("paid0001", OrderStatus.PAID, PaymentMethod.COD),
      order("cod00001", OrderStatus.PENDING, PaymentMethod.COD),
      order("onl00001", OrderStatus.PENDING, PaymentMethod.CARD),
      order("shp00001", OrderStatus.SHIPPED, PaymentMethod.COD),
      order("can00001", OrderStatus.CANCELLED, PaymentMethod.COD),
    ];
    render(
      <BulkShipSelectionProvider>
        <SellerOrdersTable orders={orders} />
      </BulkShipSelectionProvider>,
    );
    const names = screen
      .getAllByRole("checkbox")
      .map((c) => c.getAttribute("aria-label"))
      .filter((n) => n?.startsWith("Chọn đơn #"))
      .sort();
    expect(names).toEqual(["Chọn đơn #cod00001", "Chọn đơn #paid0001"]);
  });
});
