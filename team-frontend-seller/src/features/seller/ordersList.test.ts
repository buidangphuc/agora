import { describe, expect, it } from "vitest";

import { OrderStatus } from "@/generated/platform/order/v1/order_pb.js";
import * as ordersList from "./ordersList";
import {
  ORDERS_PAGE_SIZE,
  ORDER_TABS,
  tabCountsFromStatusCounts,
  tabStatuses,
} from "./ordersList";

describe("order tabs", () => {
  it("Status tab filters on the server: tabs map to server statuses", () => {
    expect(tabStatuses("pending")).toEqual([
      OrderStatus.PENDING,
      OrderStatus.PAID,
    ]);
    expect(tabStatuses("shipped")).toEqual([OrderStatus.SHIPPED]);
    expect(tabStatuses("completed")).toEqual([OrderStatus.COMPLETED]);
  });

  it("Status tab filters on the server: all and unknown tabs send no status filter", () => {
    expect(tabStatuses("all")).toEqual([]);
    expect(tabStatuses("bogus")).toEqual([]);
  });

  it("Badges stay stable while searching: tab counts come from status_counts and ALL is their sum", () => {
    expect(
      tabCountsFromStatusCounts({
        [OrderStatus.PENDING]: 3,
        [OrderStatus.PAID]: 2,
        [OrderStatus.SHIPPED]: 4,
        [OrderStatus.COMPLETED]: 5,
        [OrderStatus.CANCELLED]: 1,
      }),
    ).toEqual({ all: 15, pending: 5, shipped: 4, completed: 5 });
  });

  it("First page of a large shop: missing statuses count as 0 and 5000 orders sum to 5000", () => {
    expect(tabCountsFromStatusCounts({})).toEqual({
      all: 0,
      pending: 0,
      shipped: 0,
      completed: 0,
    });
    expect(
      tabCountsFromStatusCounts({
        [OrderStatus.PENDING]: 1000,
        [OrderStatus.PAID]: 1000,
        [OrderStatus.SHIPPED]: 1000,
        [OrderStatus.COMPLETED]: 1000,
        [OrderStatus.CANCELLED]: 1000,
      }).all,
    ).toBe(5000);
  });

  it("First page of a large shop: page size is 20 and tabs are unchanged", () => {
    expect(ORDERS_PAGE_SIZE).toBe(20);
    expect([...ORDER_TABS]).toEqual(["pending", "shipped", "completed"]);
  });

  it("Status tab filters on the server: client-side filtering helpers are gone", () => {
    const m = ordersList as Record<string, unknown>;
    expect(m.inTab).toBeUndefined();
    expect(m.tabCounts).toBeUndefined();
    expect(m.searchOrders).toBeUndefined();
  });
});
