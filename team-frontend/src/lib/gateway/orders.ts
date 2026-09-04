import "server-only";

import {
  type Order,
  type OrderItem,
  type OrderReturn,
  OrderStatus,
  ReturnStatus,
  type SagaStep,
  type Shipment,
  type ShipmentCheckpoint,
  ShipmentStatus,
} from "@/generated/platform/order/v1/order_pb.js";
import { PaymentMethod } from "@/generated/platform/payment/v1/payment_pb.js";
import { makeClients } from "./client.js";
import { getPaymentMethodText } from "./payment.js";
import { getToken } from "./session.js";

function gateway() {
  return makeClients(getToken());
}

export interface ViewOrderItem {
  id: string;
  listingId: string;
  variantId: string;
  title: string;
  variantName: string;
  quantity: number;
  unitPrice: number;
  imageUrl: string;
}

export interface ViewOrder {
  id: string;
  buyerId: string;
  sellerId: string;
  status: OrderStatus;
  statusText: string;
  totalAmount: number;
  itemsSubtotal: number;
  shippingFee: number;
  paymentMethod: PaymentMethod;
  paymentMethodText: string;
  currency: string;
  discountAmount: number;
  voucherCode: string;
  recipientName: string;
  phone: string;
  addressFull: string;
  trackingNumber: string;
  items: ViewOrderItem[];
  createdAt: string;
}

function getStatusText(s: OrderStatus): string {
  switch (s) {
    case OrderStatus.PENDING:
      return "Chờ xử lý";
    case OrderStatus.PAID:
      return "Đã thanh toán";
    case OrderStatus.SHIPPED:
      return "Đang giao hàng";
    case OrderStatus.COMPLETED:
      return "Đã hoàn thành";
    case OrderStatus.CANCELLED:
      return "Đã hủy";
    default:
      return "Không xác định";
  }
}

function mapOrderItem(it: OrderItem): ViewOrderItem {
  return {
    id: it.id,
    listingId: it.listingId,
    variantId: it.variantId,
    title: it.title,
    variantName: it.variantName,
    quantity: it.quantity,
    unitPrice: Number(it.unitPrice),
    imageUrl: it.imageUrl,
  };
}

function mapOrder(o: Order): ViewOrder {
  const addr = o.shippingAddress;
  let addressFull = "";
  if (addr) {
    const parts = [addr.street, addr.ward, addr.district, addr.city].filter(
      Boolean,
    );
    addressFull = parts.join(", ");
  }

  let createdAt = "";
  if (o.createdAt) {
    createdAt = new Date(Number(o.createdAt.seconds) * 1000).toLocaleString(
      "vi-VN",
    );
  }

  const itemsSubtotal = Number(o.itemsSubtotal) || Number(o.totalAmount);
  const shippingFee = Number(o.shippingFee) || 0;

  return {
    id: o.id,
    buyerId: o.buyerId,
    sellerId: o.sellerId,
    status: o.status,
    statusText: getStatusText(o.status),
    totalAmount: Number(o.totalAmount),
    itemsSubtotal,
    shippingFee,
    paymentMethod: o.paymentMethod,
    paymentMethodText: getPaymentMethodText(o.paymentMethod),
    currency: o.currency || "VND",
    discountAmount: Number(o.discountAmount) || 0,
    voucherCode: o.voucherCode || "",
    recipientName: addr?.recipientName ?? "",
    phone: addr?.phone ?? "",
    addressFull,
    trackingNumber: o.trackingNumber,
    items: o.items.map(mapOrderItem),
    createdAt,
  };
}

export async function calculateShippingFee(
  city: string,
  itemsSubtotal: number,
): Promise<{ shippingFee: number; isFreeShipping: boolean; message: string }> {
  try {
    const res = await gateway().order.calculateShippingFee({
      city,
      itemsSubtotal: BigInt(itemsSubtotal),
    });
    return {
      shippingFee: Number(res.shippingFee),
      isFreeShipping: res.isFreeShipping,
      message: res.message,
    };
  } catch {
    return {
      shippingFee: 20000,
      isFreeShipping: false,
      message: "Phí vận chuyển tiêu chuẩn",
    };
  }
}

export async function createOrder(
  addressId?: string,
  itemIds?: string[],
  paymentMethod?: PaymentMethod,
  voucherCode?: string,
): Promise<ViewOrder[]> {
  const res = await gateway().order.createOrder({
    addressId: addressId ?? "",
    itemIds: itemIds ?? [],
    paymentMethod: paymentMethod ?? PaymentMethod.COD,
    voucherCode: voucherCode ?? "",
  });
  return res.orders.map(mapOrder);
}

export async function getOrder(id: string): Promise<ViewOrder | null> {
  try {
    const res = await gateway().order.getOrder({ id });
    return res.order ? mapOrder(res.order) : null;
  } catch {
    return null;
  }
}

export async function listBuyerOrders(
  statusFilter: OrderStatus = OrderStatus.UNSPECIFIED,
): Promise<ViewOrder[]> {
  try {
    const res = await gateway().order.listBuyerOrders({ statusFilter });
    return res.orders.map(mapOrder);
  } catch {
    return [];
  }
}

export async function listSellerOrders(
  statusFilter: OrderStatus = OrderStatus.UNSPECIFIED,
): Promise<ViewOrder[]> {
  try {
    const res = await gateway().order.listSellerOrders({ statusFilter });
    return res.orders.map(mapOrder);
  } catch {
    return [];
  }
}

export async function updateOrderStatus(
  id: string,
  status: OrderStatus,
  trackingNumber?: string,
): Promise<ViewOrder> {
  const res = await gateway().order.updateOrderStatus({
    id,
    status,
    trackingNumber: trackingNumber ?? "",
  });
  if (!res.order) throw new Error("update order status failed");
  return mapOrder(res.order);
}

export async function cancelOrder(
  id: string,
  reason?: string,
): Promise<ViewOrder> {
  const res = await gateway().order.cancelOrder({
    id,
    reason: reason ?? "",
  });
  if (!res.order) throw new Error("cancel order failed");
  return mapOrder(res.order);
}

// ---------------------------------------------------------------------------
// Post-purchase: shipment tracking timeline + saga steps
// ---------------------------------------------------------------------------

export interface ViewShipmentCheckpoint {
  timestamp: string;
  location: string;
  description: string;
}

export interface ViewShipment {
  id: string;
  carrier: string;
  trackingCode: string;
  status: ShipmentStatus;
  statusText: string;
  checkpoints: ViewShipmentCheckpoint[];
}

export interface ViewSagaStep {
  name: string;
  status: string;
  detail: string;
  timestamp: string;
}

function fmtTs(ts?: { seconds: bigint }): string {
  if (!ts) return "";
  return new Date(Number(ts.seconds) * 1000).toLocaleString("vi-VN");
}

function getShipmentStatusText(s: ShipmentStatus): string {
  switch (s) {
    case ShipmentStatus.PENDING:
      return "Chờ lấy hàng";
    case ShipmentStatus.PICKED_UP:
      return "Đã lấy hàng";
    case ShipmentStatus.IN_TRANSIT:
      return "Đang vận chuyển";
    case ShipmentStatus.DELIVERED:
      return "Đã giao hàng";
    case ShipmentStatus.FAILED:
      return "Giao hàng thất bại";
    default:
      return "Chưa xác định";
  }
}

function mapCheckpoint(c: ShipmentCheckpoint): ViewShipmentCheckpoint {
  return {
    timestamp: fmtTs(c.timestamp),
    location: c.location,
    description: c.description,
  };
}

function mapShipment(s: Shipment): ViewShipment {
  return {
    id: s.id,
    carrier: s.carrier,
    trackingCode: s.trackingCode,
    status: s.status,
    statusText: getShipmentStatusText(s.status),
    checkpoints: s.checkpoints.map(mapCheckpoint),
  };
}

function mapSagaStep(s: SagaStep): ViewSagaStep {
  return {
    name: s.name,
    status: s.status,
    detail: s.detail,
    timestamp: fmtTs(s.timestamp),
  };
}

/** Fetch the shipment (carrier + checkpoints) for an order's tracking timeline.
 * Returns null when no shipment exists yet or the call fails. */
export async function getShipmentTracking(
  orderId: string,
): Promise<ViewShipment | null> {
  try {
    const res = await gateway().order.getShipmentTracking({ orderId });
    return res.shipment ? mapShipment(res.shipment) : null;
  } catch {
    return null;
  }
}

/** Fetch the order saga steps (order → stock → payment → confirm) for the
 * fulfillment timeline. Returns [] on failure. */
export async function getSagaState(orderId: string): Promise<ViewSagaStep[]> {
  try {
    const res = await gateway().order.getSagaState({ orderId });
    return res.steps.map(mapSagaStep);
  } catch {
    return [];
  }
}

// ---------------------------------------------------------------------------
// Post-purchase: returns / refunds
// ---------------------------------------------------------------------------

export interface ViewOrderReturn {
  id: string;
  orderId: string;
  reason: string;
  refundAmount: number;
  status: ReturnStatus;
  statusText: string;
}

export function getReturnStatusText(s: ReturnStatus): string {
  switch (s) {
    case ReturnStatus.PENDING:
      return "Chờ duyệt";
    case ReturnStatus.APPROVED:
      return "Đã duyệt";
    case ReturnStatus.REJECTED:
      return "Đã từ chối";
    case ReturnStatus.REFUNDED:
      return "Đã hoàn tiền";
    default:
      return "Chưa xác định";
  }
}

function mapReturn(r: OrderReturn): ViewOrderReturn {
  return {
    id: r.id,
    orderId: r.orderId,
    reason: r.reason,
    refundAmount: Number(r.refundAmount),
    status: r.status,
    statusText: getReturnStatusText(r.status),
  };
}

export async function createReturnRequest(
  orderId: string,
  reason: string,
  refundAmount: number,
): Promise<ViewOrderReturn> {
  const res = await gateway().order.createReturnRequest({
    orderId,
    reason,
    refundAmount: BigInt(refundAmount),
  });
  if (!res.returnRequest) throw new Error("create return request failed");
  return mapReturn(res.returnRequest);
}

export async function getReturnRequest(
  id: string,
): Promise<ViewOrderReturn | null> {
  try {
    const res = await gateway().order.getReturnRequest({ id });
    return res.returnRequest ? mapReturn(res.returnRequest) : null;
  } catch {
    return null;
  }
}

export async function updateReturnStatus(
  id: string,
  status: ReturnStatus,
): Promise<ViewOrderReturn> {
  const res = await gateway().order.updateReturnStatus({ id, status });
  if (!res.returnRequest) throw new Error("update return status failed");
  return mapReturn(res.returnRequest);
}
