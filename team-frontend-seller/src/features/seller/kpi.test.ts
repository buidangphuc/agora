import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { OrderStatus } from "@/generated/platform/order/v1/order_pb.js";
import { PaymentMethod } from "@/generated/platform/payment/v1/payment_pb.js";
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
const order = (
  status: OrderStatus,
  paymentMethod = PaymentMethod.MOCK_MOMO,
  shipBy = "",
) => ({ status, paymentMethod, shipBy }) as ViewOrder;

const NOW = new Date("2026-10-17T10:00:00.000Z");
const at = (offsetMs: number) =>
  new Date(NOW.getTime() + offsetMs).toISOString();

beforeEach(() => {
  vi.useFakeTimers();
  vi.setSystemTime(NOW);
});
afterEach(() => vi.useRealTimers());

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
        order(OrderStatus.PENDING, PaymentMethod.COD),
        order(OrderStatus.PAID),
        order(OrderStatus.SHIPPED),
        order(OrderStatus.COMPLETED),
      ],
    });
    expect(byKey(kpis)).toEqual({
      total: 3,
      published: 3,
      "low-stock": 1,
      "to-ship": 2,
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
    expect(noListings.map((k) => k.key)).toEqual(["to-ship"]);
    expect(deriveWorkplaceKpis({ listings: null, orders: null })).toEqual([]);
  });

  it("shows zero only because the source returned nothing", () => {
    const kpis = deriveWorkplaceKpis({ listings: listings([]), orders: [] });
    expect(kpis.map((k) => k.value)).toEqual([0, 0, 0, 0]);
  });
});

describe("[Order lifecycle: the to-ship count is paid plus COD pending] to-ship KPI", () => {
  const kpisFor = (orders: ViewOrder[]) =>
    deriveWorkplaceKpis({ listings: null, orders });

  it("to-ship counts paid and cod pending", () => {
    const kpis = kpisFor([
      order(OrderStatus.PAID),
      order(OrderStatus.PAID),
      order(OrderStatus.PENDING, PaymentMethod.COD),
      order(OrderStatus.PENDING, PaymentMethod.MOCK_BANK),
      order(OrderStatus.SHIPPED),
      order(OrderStatus.CANCELLED),
    ]);
    const toShip = kpis.find((k) => k.key === "to-ship");
    expect(toShip).toMatchObject({ title: "Cần gửi", value: 3 });
    expect(kpis.find((k) => k.title === "Đơn chờ xử lý")).toBeUndefined();
    expect(kpis.find((k) => k.key === "open-orders")).toBeUndefined();
  });

  it("never counts a pending online-pay order of any online method", () => {
    const kpis = kpisFor([
      order(OrderStatus.PENDING, PaymentMethod.MOCK_MOMO),
      order(OrderStatus.PENDING, PaymentMethod.MOCK_CARD),
      order(OrderStatus.PENDING, PaymentMethod.MOCK_BANK),
    ]);
    expect(kpis.find((k) => k.key === "to-ship")?.value).toBe(0);
  });
});

describe("[Order lifecycle: overdue shipments are highlighted] overdue KPI", () => {
  const toShip = (orders: ViewOrder[]) =>
    deriveWorkplaceKpis({ listings: null, orders }).find(
      (k) => k.key === "to-ship",
    );

  it("overdue is strictly past ship-by", () => {
    const k = toShip([
      order(OrderStatus.PAID, PaymentMethod.MOCK_MOMO, at(-3_600_000)),
      order(OrderStatus.PAID, PaymentMethod.MOCK_MOMO, at(0)),
      order(OrderStatus.PAID, PaymentMethod.MOCK_MOMO, at(3_600_000)),
    ]);
    expect(k?.value).toBe(3);
    expect(k?.note).toBe("Quá hạn: 1");
    expect(k?.tone).toBe("danger");
  });

  it("has no overdue note and no danger tone when nothing is past ship-by", () => {
    const k = toShip([
      order(OrderStatus.PAID, PaymentMethod.MOCK_MOMO, at(0)),
      order(OrderStatus.PAID, PaymentMethod.MOCK_MOMO, at(3_600_000)),
      order(OrderStatus.PAID),
    ]);
    expect(k?.note).toBeUndefined();
    expect(k?.tone).not.toBe("danger");
  });

  it("does not count a past ship-by on an order that is no longer to-ship", () => {
    const k = toShip([
      order(OrderStatus.SHIPPED, PaymentMethod.MOCK_MOMO, at(-3_600_000)),
      order(OrderStatus.PENDING, PaymentMethod.MOCK_BANK, at(-3_600_000)),
    ]);
    expect(k?.value).toBe(0);
    expect(k?.note).toBeUndefined();
  });
});
