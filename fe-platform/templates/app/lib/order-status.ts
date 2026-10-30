import { OrderStatus } from "@fe-platform/api-client";

const LABELS: Partial<Record<OrderStatus, string>> = {
  [OrderStatus.PENDING]: "Waiting for payment",
  [OrderStatus.PAID]: "Paid",
  [OrderStatus.SHIPPED]: "Shipped",
  [OrderStatus.COMPLETED]: "Completed",
  [OrderStatus.CANCELLED]: "Cancelled",
};

export function orderStatusLabel(status: OrderStatus): string {
  return LABELS[status] ?? OrderStatus[status] ?? "Unknown";
}
