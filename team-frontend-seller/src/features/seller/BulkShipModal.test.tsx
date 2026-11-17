import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { bulkShipOrdersAction } from "@/features/order/actions";
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

beforeEach(() => vi.clearAllMocks());

function order(id: string): ViewOrder {
  return {
    id,
    buyerId: "b",
    sellerId: "me",
    status: OrderStatus.PAID,
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

describe("BulkShipModal", () => {
  it("Bulk ship: failed rows stay selected and shipped rows leave: failed rows stay selected", async () => {
    const orders = [order("ord00001"), order("ord00002"), order("ord00003")];
    vi.mocked(bulkShipOrdersAction).mockResolvedValue({
      ok: true,
      data: {
        results: [
          { orderId: "ord00001", ok: true },
          { orderId: "ord00002", ok: false, error: "Đơn đã bị huỷ" },
          { orderId: "ord00003", ok: true },
        ],
      },
    } as never);
    render(
      <BulkShipSelectionProvider>
        <BulkShipBar orders={orders} />
        <SellerOrdersTable orders={orders} />
      </BulkShipSelectionProvider>,
    );
    fireEvent.click(
      screen.getByRole("checkbox", { name: "Chọn tất cả đơn có thể gửi" }),
    );
    fireEvent.click(screen.getByRole("button", { name: "Gửi hàng loạt" }));
    fireEvent.change(screen.getByLabelText("Mã vận đơn #ord00001"), {
      target: { value: "  T1  " },
    });
    fireEvent.click(
      screen.getByRole("button", { name: "Xác nhận gửi hàng loạt" }),
    );

    await waitFor(() =>
      expect(screen.getByText("Đơn đã bị huỷ")).toBeInTheDocument(),
    );
    expect(bulkShipOrdersAction).toHaveBeenCalledTimes(1);
    const sent = vi.mocked(bulkShipOrdersAction).mock.calls[0][0];
    expect(sent.map((o) => o.orderId)).toEqual([
      "ord00001",
      "ord00002",
      "ord00003",
    ]);
    expect(sent[0].trackingNumber?.trim()).toBe("T1");
    expect(sent[1].trackingNumber || undefined).toBeUndefined();

    expect(
      screen.getByRole("checkbox", { name: "Chọn đơn #ord00002" }),
    ).toBeChecked();
    expect(
      screen.getByRole("checkbox", { name: "Chọn đơn #ord00001" }),
    ).not.toBeChecked();
    expect(
      screen.getByRole("checkbox", { name: "Chọn đơn #ord00003" }),
    ).not.toBeChecked();
    expect(screen.getByText("Đã chọn 1 đơn")).toBeInTheDocument();
  });
});
