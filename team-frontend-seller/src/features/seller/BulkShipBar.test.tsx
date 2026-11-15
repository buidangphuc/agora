import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { OrderStatus } from "@/generated/platform/order/v1/order_pb.js";
import { PaymentMethod } from "@/generated/platform/payment/v1/payment_pb.js";
import type { ViewOrder } from "@/lib/gateway/orders";
import { BulkShipBar } from "./BulkShipBar";
import { BulkShipSelectionProvider } from "./BulkShipSelection";
import { SellerOrdersTable } from "./SellerOrdersTable";

vi.mock("@/features/order/actions", () => ({
  updateOrderStatusAction: vi.fn(),
  bulkShipOrdersAction: vi.fn(),
}));
vi.mock("@/components/ui/ToastProvider", () => ({
  useToast: () => ({ success: vi.fn(), error: vi.fn() }),
}));

function order(id: string, status: OrderStatus): ViewOrder {
  return {
    id,
    buyerId: "b",
    sellerId: "me",
    status,
    statusText: "x",
    totalAmount: 1000,
    itemsSubtotal: 1000,
    shippingFee: 0,
    paymentMethod: PaymentMethod.COD,
    paymentMethodText: "COD",
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

function mount(orders: ViewOrder[]) {
  render(
    <BulkShipSelectionProvider>
      <BulkShipBar orders={orders} />
      <SellerOrdersTable orders={orders} />
    </BulkShipSelectionProvider>,
  );
}

describe("BulkShipBar", () => {
  it("Bulk ship: select-all selects the shippable rows of the page: select all picks shippable rows", () => {
    const orders = [
      ...Array.from({ length: 8 }, (_, i) =>
        order(`pay${String(i).padStart(5, "0")}`, OrderStatus.PAID),
      ),
      ...Array.from({ length: 12 }, (_, i) =>
        order(`shp${String(i).padStart(5, "0")}`, OrderStatus.SHIPPED),
      ),
    ];
    mount(orders);
    fireEvent.click(
      screen.getByRole("checkbox", { name: "Chọn tất cả đơn có thể gửi" }),
    );
    expect(screen.getByText("Đã chọn 8 đơn")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Gửi hàng loạt" })).toBeEnabled();
    expect(
      screen.getByRole("button", { name: "In phiếu hàng loạt" }),
    ).toBeEnabled();
  });

  it("Bulk ship: nothing selected disables both buttons: buttons disabled when empty", () => {
    mount([order("pay00001", OrderStatus.PAID)]);
    expect(
      screen.getByRole("button", { name: "Gửi hàng loạt" }),
    ).toBeDisabled();
    expect(
      screen.getByRole("button", { name: "In phiếu hàng loạt" }),
    ).toBeDisabled();
    fireEvent.click(
      screen.getByRole("checkbox", { name: "Chọn đơn #pay00001" }),
    );
    expect(screen.getByRole("button", { name: "Gửi hàng loạt" })).toBeEnabled();
    fireEvent.click(
      screen.getByRole("checkbox", { name: "Chọn đơn #pay00001" }),
    );
    expect(
      screen.getByRole("button", { name: "Gửi hàng loạt" }),
    ).toBeDisabled();
  });
});
