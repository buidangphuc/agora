import { describe, expect, it } from "vitest";

import { OrderStatus } from "@/generated/platform/order/v1/order_pb.js";
import { PaymentMethod } from "@/generated/platform/payment/v1/payment_pb.js";
import { canPayAgain, canReturn } from "./lifecycle";
import { makeOrder } from "./orderFixtures";

const DUE = "2026-10-17T10:00:00.000Z";
const dueMs = Date.parse(DUE);

function pending(over: Parameters<typeof makeOrder>[2] = {}) {
  return makeOrder("o1", OrderStatus.PENDING, {
    paymentMethod: PaymentMethod.MOCK_MOMO,
    ...over,
  });
}

describe("lifecycle rules (pure, explicit now)", () => {
  // Scenario: Order lifecycle: the pay action disappears at the deadline instant
  it("canPayAgain flips at the deadline instant", () => {
    const order = pending({ paymentDueAt: DUE });
    expect(canPayAgain(order, new Date(dueMs - 1))).toBe(true);
    expect(canPayAgain(order, new Date(dueMs))).toBe(false);
    expect(canPayAgain(order, new Date(dueMs + 1))).toBe(false);
  });

  // Scenario: Order lifecycle: an order with no deadline keeps its pay action
  it("legacy order without deadline can still be paid", () => {
    const order = pending({ paymentDueAt: "" });
    expect(canPayAgain(order, new Date(dueMs))).toBe(true);
    expect(canPayAgain(order, new Date("2030-01-01T00:00:00Z"))).toBe(true);
  });

  // Supports the pay-action scenarios: COD and non-pending never offer pay.
  it("no pay action for COD or non-pending orders", () => {
    const now = new Date(dueMs - 1000);
    expect(
      canPayAgain(
        pending({ paymentMethod: PaymentMethod.COD, paymentDueAt: DUE }),
        now,
      ),
    ).toBe(false);
    expect(
      canPayAgain(
        makeOrder("o2", OrderStatus.PAID, {
          paymentMethod: PaymentMethod.MOCK_MOMO,
        }),
        now,
      ),
    ).toBe(false);
  });

  // Scenario: Order lifecycle: a return is offered for shipped orders and inside the window
  it("return offered when shipped or inside the window", () => {
    const now = new Date("2026-10-17T00:00:00.000Z");
    const shipped = makeOrder("o3", OrderStatus.SHIPPED);
    const completed = makeOrder("o4", OrderStatus.COMPLETED, {
      returnWindowEndsAt: "2026-10-18T00:00:00.000Z",
    });
    expect(canReturn(shipped, now)).toBe(true);
    expect(canReturn(completed, now)).toBe(true);
  });

  // Scenario: Order lifecycle: a return is not offered on a paid order
  it("no return before shipping", () => {
    const now = new Date("2026-10-17T00:00:00.000Z");
    for (const s of [
      OrderStatus.PAID,
      OrderStatus.PENDING,
      OrderStatus.CANCELLED,
    ]) {
      expect(canReturn(makeOrder("o5", s), now)).toBe(false);
    }
  });

  // Scenario: Order lifecycle: the return window closes at its end instant
  it("return window closes at its end instant", () => {
    const end = "2026-11-01T00:00:00.000Z";
    const endMs = Date.parse(end);
    const order = makeOrder("o6", OrderStatus.COMPLETED, {
      returnWindowEndsAt: end,
    });
    expect(canReturn(order, new Date(endMs - 1))).toBe(true);
    expect(canReturn(order, new Date(endMs))).toBe(false);
  });

  // Scenario: Order lifecycle: a completed order without a window leaves the decision to the server
  it("completed order without window is offered", () => {
    const order = makeOrder("o7", OrderStatus.COMPLETED, {
      returnWindowEndsAt: "",
    });
    expect(canReturn(order, new Date("2030-01-01T00:00:00Z"))).toBe(true);
  });
});
