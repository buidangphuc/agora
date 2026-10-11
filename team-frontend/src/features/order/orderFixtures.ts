import { OrderStatus } from "@/generated/platform/order/v1/order_pb.js";
import { PaymentMethod } from "@/generated/platform/payment/v1/payment_pb.js";
import type { ViewOrder } from "@/lib/gateway/orders";

const STATUS_TEXT: Record<number, string> = {
  [OrderStatus.PENDING]: "Chờ xử lý",
  [OrderStatus.PAID]: "Đã thanh toán",
  [OrderStatus.SHIPPED]: "Đang giao hàng",
  [OrderStatus.COMPLETED]: "Đã hoàn thành",
  [OrderStatus.CANCELLED]: "Đã hủy",
};

/** Test fixture: a buyer order with one item. */
export function makeOrder(
  id: string,
  status: OrderStatus = OrderStatus.PENDING,
  over: Partial<ViewOrder> = {},
): ViewOrder {
  return {
    id,
    buyerId: "b1",
    sellerId: "seller-abcdef-123",
    status,
    statusText: STATUS_TEXT[status] ?? "Không xác định",
    totalAmount: 120000,
    itemsSubtotal: 100000,
    shippingFee: 20000,
    paymentMethod: PaymentMethod.COD,
    paymentMethodText: "Thanh toán khi nhận hàng",
    currency: "VND",
    discountAmount: 0,
    voucherCode: "",
    recipientName: "An",
    phone: "0900000000",
    addressFull: "1 Main, HCM",
    trackingNumber: "",
    createdAt: "01/09/2026",
    paidAt: "",
    items: [
      {
        id: `${id}-it1`,
        listingId: "l1",
        variantId: "v1",
        title: "Áo thun",
        variantName: "Đỏ",
        quantity: 2,
        unitPrice: 50000,
        imageUrl: "a.png",
      },
    ],
    ...over,
  };
}
