import { OrderStatus } from "@/generated/platform/order/v1/order_pb.js";
import type { ViewOrder } from "@/lib/gateway/orders";

export const ORDERS_PAGE_SIZE = 10;

export type OrderStatusKey =
  | "all"
  | "pending"
  | "paid"
  | "shipped"
  | "completed"
  | "cancelled";

export const ORDER_STATUS_TABS: { id: OrderStatusKey; label: string }[] = [
  { id: "all", label: "Tất cả" },
  { id: "pending", label: "Chờ xử lý" },
  { id: "paid", label: "Đã thanh toán" },
  { id: "shipped", label: "Đang giao" },
  { id: "completed", label: "Đã giao" },
  { id: "cancelled", label: "Đã hủy" },
];

const STATUS_OF: Record<Exclude<OrderStatusKey, "all">, OrderStatus> = {
  pending: OrderStatus.PENDING,
  paid: OrderStatus.PAID,
  shipped: OrderStatus.SHIPPED,
  completed: OrderStatus.COMPLETED,
  cancelled: OrderStatus.CANCELLED,
};

/** An unknown or missing `status` query value behaves as "all". */
export function parseOrderStatus(value: string | undefined): OrderStatusKey {
  return ORDER_STATUS_TABS.some((t) => t.id === value)
    ? (value as OrderStatusKey)
    : "all";
}

export interface OrdersPage {
  status: OrderStatusKey;
  /** The page actually shown (1-based, clamped). */
  page: number;
  pageCount: number;
  /** Orders in the selected tab (all pages). */
  total: number;
  orders: ViewOrder[];
  counts: Record<OrderStatusKey, number>;
}

/**
 * Filter by status tab, count every tab, clamp the page and slice 10 orders.
 * `status` and `page` come straight from searchParams, so both are validated here.
 */
export function paginateOrders(
  orders: ViewOrder[],
  status: string | undefined,
  page: string | number | undefined,
): OrdersPage {
  const key = parseOrderStatus(status);
  const counts: Record<OrderStatusKey, number> = {
    all: orders.length,
    pending: 0,
    paid: 0,
    shipped: 0,
    completed: 0,
    cancelled: 0,
  };
  for (const o of orders) {
    for (const [k, s] of Object.entries(STATUS_OF)) {
      if (o.status === s) counts[k as OrderStatusKey] += 1;
    }
  }

  const filtered =
    key === "all" ? orders : orders.filter((o) => o.status === STATUS_OF[key]);
  const pageCount = Math.max(1, Math.ceil(filtered.length / ORDERS_PAGE_SIZE));
  const requested = Math.trunc(Number(page));
  const current = Math.min(
    Math.max(1, Number.isFinite(requested) ? requested : 1),
    pageCount,
  );
  const start = (current - 1) * ORDERS_PAGE_SIZE;

  return {
    status: key,
    page: current,
    pageCount,
    total: filtered.length,
    orders: filtered.slice(start, start + ORDERS_PAGE_SIZE),
    counts,
  };
}
