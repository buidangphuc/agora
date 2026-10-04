import { describe, expect, it } from "vitest";

import { OrderStatus } from "@/generated/platform/order/v1/order_pb.js";
import type { ListingPage } from "@/lib/gateway/listings";
import type { ViewOrder } from "@/lib/gateway/orders";
import { deriveWorkplaceKpis } from "./kpi";

const listing = (id: string, status: string, stock: number) => ({
  id,
  status,
  stock,
});
const listings = (items: ReturnType<typeof listing>[], total = items.length) =>
  ({ items, nextCursor: "", total }) as unknown as ListingPage;
const order = (status: OrderStatus) => ({ status }) as ViewOrder;

const byKey = (k: ReturnType<typeof deriveWorkplaceKpis>) =>
  Object.fromEntries(k.map((x) => [x.key, x.value]));

describe("deriveWorkplaceKpis", () => {
  it("derives every KPI from the fetched data", () => {
    const kpis = deriveWorkplaceKpis({
      listings: listings([
        listing("a", "published", 10),
        listing("b", "published", 2),
        listing("c", "published", 40),
      ]),
      orders: [
        order(OrderStatus.PENDING),
        order(OrderStatus.PAID),
        order(OrderStatus.SHIPPED),
        order(OrderStatus.COMPLETED),
      ],
    });
    expect(byKey(kpis)).toEqual({
      total: 3,
      published: 3,
      "low-stock": 1,
      "open-orders": 2,
    });
  });

  it("uses the response total, not the page length", () => {
    const kpis = deriveWorkplaceKpis({
      listings: listings([listing("a", "draft", 9)], 45),
      orders: [],
    });
    expect(byKey(kpis)).toMatchObject({ total: 45, published: 0 });
  });

  it("hides a KPI whose source failed instead of zeroing it", () => {
    const noOrders = deriveWorkplaceKpis({
      listings: listings([listing("a", "published", 9)]),
      orders: null,
    });
    expect(noOrders.map((k) => k.key)).toEqual([
      "total",
      "published",
      "low-stock",
    ]);

    const noListings = deriveWorkplaceKpis({
      listings: null,
      orders: [order(OrderStatus.PENDING)],
    });
    expect(noListings.map((k) => k.key)).toEqual(["open-orders"]);
    expect(deriveWorkplaceKpis({ listings: null, orders: null })).toEqual([]);
  });

  it("shows zero only because the source returned nothing", () => {
    const kpis = deriveWorkplaceKpis({ listings: listings([]), orders: [] });
    expect(kpis.map((k) => k.value)).toEqual([0, 0, 0, 0]);
  });
});
