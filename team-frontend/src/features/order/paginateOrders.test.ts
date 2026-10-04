import { describe, expect, it } from "vitest";

import { OrderStatus } from "@/generated/platform/order/v1/order_pb.js";
import type { ViewOrder } from "@/lib/gateway/orders";

import { paginateOrders, parseOrderStatus } from "./paginateOrders";

function order(id: string, status: OrderStatus): ViewOrder {
  return { id, status } as ViewOrder;
}

const orders: ViewOrder[] = [
  ...Array.from({ length: 12 }, (_, i) =>
    order(`c${i}`, OrderStatus.COMPLETED),
  ),
  ...Array.from({ length: 8 }, (_, i) => order(`p${i}`, OrderStatus.PAID)),
  order("x0", OrderStatus.CANCELLED),
  order("x1", OrderStatus.PENDING),
  order("x2", OrderStatus.SHIPPED),
];

describe("paginateOrders", () => {
  it("counts every tab and slices 10 per page", () => {
    const res = paginateOrders(orders, "all", 1);
    expect(res.orders).toHaveLength(10);
    expect(res.pageCount).toBe(3);
    expect(res.total).toBe(23);
    expect(res.counts).toEqual({
      all: 23,
      pending: 1,
      paid: 8,
      shipped: 1,
      completed: 12,
      cancelled: 1,
    });
  });

  it("shows the last orders on the last page", () => {
    const res = paginateOrders(orders, "all", "3");
    expect(res.page).toBe(3);
    expect(res.orders).toHaveLength(3);
  });

  it("filters by status and keeps all counts", () => {
    const res = paginateOrders(orders, "completed", 2);
    expect(res.total).toBe(12);
    expect(res.orders.map((o) => o.id)).toEqual(["c10", "c11"]);
    expect(res.counts.paid).toBe(8);
  });

  it("falls back to all for an invalid status", () => {
    expect(paginateOrders(orders, "bogus", 1).status).toBe("all");
    expect(paginateOrders(orders, undefined, 1).total).toBe(23);
    expect(parseOrderStatus("paid")).toBe("paid");
  });

  it("clamps an out-of-range or invalid page", () => {
    expect(paginateOrders(orders, "all", 99).page).toBe(3);
    expect(paginateOrders(orders, "all", 0).page).toBe(1);
    expect(paginateOrders(orders, "all", -4).page).toBe(1);
    expect(paginateOrders(orders, "all", "abc").page).toBe(1);
    expect(paginateOrders(orders, "all", undefined).page).toBe(1);
  });

  it("returns one empty page for no orders", () => {
    const res = paginateOrders([], "cancelled", 5);
    expect(res).toMatchObject({ page: 1, pageCount: 1, total: 0, orders: [] });
    expect(res.counts.all).toBe(0);
  });
});
