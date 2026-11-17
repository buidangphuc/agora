import { OrderStatus } from "@/generated/platform/order/v1/order_pb.js";

export const ORDER_TABS = ["pending", "shipped", "completed"] as const;
export type OrderTab = (typeof ORDER_TABS)[number] | "all";

export const ORDERS_PAGE_SIZE = 20;

const MATCH: Record<(typeof ORDER_TABS)[number], OrderStatus[]> = {
  pending: [OrderStatus.PENDING, OrderStatus.PAID],
  shipped: [OrderStatus.SHIPPED],
  completed: [OrderStatus.COMPLETED],
};

export function tabStatuses(tab: string): OrderStatus[] {
  return MATCH[tab as (typeof ORDER_TABS)[number]] ?? [];
}

export function tabCountsFromStatusCounts(
  counts: Partial<Record<OrderStatus, number>>,
): Record<OrderTab, number> {
  const n = (s: OrderStatus) => counts[s] ?? 0;
  return {
    all:
      n(OrderStatus.PENDING) +
      n(OrderStatus.PAID) +
      n(OrderStatus.SHIPPED) +
      n(OrderStatus.COMPLETED) +
      n(OrderStatus.CANCELLED),
    pending: n(OrderStatus.PENDING) + n(OrderStatus.PAID),
    shipped: n(OrderStatus.SHIPPED),
    completed: n(OrderStatus.COMPLETED),
  };
}
