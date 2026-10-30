import { describe, expect, it } from "vitest";

import { OrderStatus } from "@/generated/platform/order/v1/order_pb.js";
import type { ViewOrder } from "@/lib/gateway/orders";
import { inTab, searchOrders, tabCounts } from "./ordersList";

const o = (id: string, status: OrderStatus, extra: Partial<ViewOrder> = {}) =>
  ({
    id,
    status,
    recipientName: "",
    phone: "",
    items: [],
    ...extra,
  }) as ViewOrder;

const orders = [
  o("a1", OrderStatus.PENDING, { recipientName: "An" }),
  o("b2", OrderStatus.PAID, { phone: "0912345678" }),
  o("c3", OrderStatus.SHIPPED, { items: [{ title: "Áo thun" } as never] }),
  o("d4", OrderStatus.COMPLETED),
  o("e5", OrderStatus.CANCELLED),
];

describe("order tabs", () => {
  it("groups pending and paid under Chờ xử lý", () => {
    expect(inTab(orders, "pending").map((x) => x.id)).toEqual(["a1", "b2"]);
    expect(inTab(orders, "shipped").map((x) => x.id)).toEqual(["c3"]);
    expect(inTab(orders, "all")).toHaveLength(5);
    expect(inTab(orders, "bogus")).toHaveLength(5);
  });

  it("counts every tab from the one list", () => {
    expect(tabCounts(orders)).toEqual({
      all: 5,
      pending: 2,
      shipped: 1,
      completed: 1,
    });
  });
});

describe("searchOrders", () => {
  it("matches id prefix, recipient, phone and item title", () => {
    expect(searchOrders(orders, "c3")).toHaveLength(1);
    expect(searchOrders(orders, "an")[0].id).toBe("a1");
    expect(searchOrders(orders, "0912")[0].id).toBe("b2");
    expect(searchOrders(orders, "áo")[0].id).toBe("c3");
    expect(searchOrders(orders, "  ")).toHaveLength(5);
  });
});
