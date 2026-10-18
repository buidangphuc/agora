import { OrderStatus } from "@/generated/platform/order/v1/order_pb.js";
import { PaymentMethod } from "@/generated/platform/payment/v1/payment_pb.js";
import type { ViewOrder } from "@/lib/gateway/orders";

/** Empty or unparsable ISO string -> null (unset). */
function instant(iso: string): number | null {
  if (!iso) return null;
  const ms = Date.parse(iso);
  return Number.isNaN(ms) ? null : ms;
}

/** Pay action: pending online-paid order whose deadline (if any) has not passed. */
export function canPayAgain(order: ViewOrder, now: Date): boolean {
  if (order.status !== OrderStatus.PENDING) return false;
  if (order.paymentMethod === PaymentMethod.COD) return false;
  const due = instant(order.paymentDueAt);
  return due === null || now.getTime() < due;
}

/** Return action: shipped, or completed inside the window (no window = server decides). */
export function canReturn(order: ViewOrder, now: Date): boolean {
  if (order.status === OrderStatus.SHIPPED) return true;
  if (order.status !== OrderStatus.COMPLETED) return false;
  const end = instant(order.returnWindowEndsAt);
  return end === null || now.getTime() < end;
}
