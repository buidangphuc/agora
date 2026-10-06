import { OrderStatus } from "@/generated/platform/order/v1/order_pb.js";
import type { ViewOrder } from "@/lib/gateway/orders";

export const ORDER_TABS = ["pending", "shipped", "completed"] as const;
export type OrderTab = (typeof ORDER_TABS)[number] | "all";

export const ORDERS_PAGE_SIZE = 20;

const MATCH: Record<(typeof ORDER_TABS)[number], OrderStatus[]> = {
  // Orders waiting on the seller: not yet handed to the carrier.
  pending: [OrderStatus.PENDING, OrderStatus.PAID],
  shipped: [OrderStatus.SHIPPED],
  completed: [OrderStatus.COMPLETED],
};

/** Orders in a status tab ("all" keeps everything). */
export function inTab(orders: ViewOrder[], tab: string): ViewOrder[] {
  const statuses = MATCH[tab as (typeof ORDER_TABS)[number]];
  return statuses ? orders.filter((o) => statuses.includes(o.status)) : orders;
}

/** Counts shown on the tabs, from the one full order list. */
export function tabCounts(orders: ViewOrder[]): Record<OrderTab, number> {
  return {
    all: orders.length,
    pending: inTab(orders, "pending").length,
    shipped: inTab(orders, "shipped").length,
    completed: inTab(orders, "completed").length,
  };
}

/** Text search over order id, recipient, phone and item titles. */
export function searchOrders(orders: ViewOrder[], q: string): ViewOrder[] {
  const needle = q.trim().toLowerCase();
  if (!needle) return orders;
  return orders.filter(
    (o) =>
      o.id.toLowerCase().startsWith(needle) ||
      o.recipientName.toLowerCase().includes(needle) ||
      o.phone.includes(needle) ||
      o.items.some((it) => it.title.toLowerCase().includes(needle)),
  );
}
