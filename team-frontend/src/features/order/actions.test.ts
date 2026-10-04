import { beforeEach, describe, expect, it, vi } from "vitest";

import { OrderStatus } from "@/generated/platform/order/v1/order_pb.js";
import { PaymentMethod } from "@/generated/platform/payment/v1/payment_pb.js";
import {
  cancelOrder,
  createOrder,
  updateOrderStatus,
} from "@/lib/gateway/orders";
import { createPayment, processMockPayment } from "@/lib/gateway/payment";

import {
  cancelOrderAction,
  checkoutAction,
  createPaymentAction,
  processMockPaymentAction,
  updateOrderStatusAction,
} from "./actions";

vi.mock("@/lib/gateway/orders", () => ({
  createOrder: vi.fn(),
  updateOrderStatus: vi.fn(),
  cancelOrder: vi.fn(),
}));
vi.mock("@/lib/gateway/payment", () => ({
  createPayment: vi.fn(),
  processMockPayment: vi.fn(),
}));

beforeEach(() => vi.clearAllMocks());

describe("checkoutAction", () => {
  it("creates orders and skips payment for COD", async () => {
    vi.mocked(createOrder).mockResolvedValue([{ id: "o1" }] as never);
    const res = await checkoutAction("addr1", ["it1"], PaymentMethod.COD);
    expect(createOrder).toHaveBeenCalledWith(
      "addr1",
      ["it1"],
      PaymentMethod.COD,
      undefined,
    );
    expect(createPayment).not.toHaveBeenCalled();
    expect(res).toEqual({
      ok: true,
      data: { orderIds: ["o1"], paymentUrl: undefined },
    });
  });

  it("initiates a payment for an online method", async () => {
    vi.mocked(createOrder).mockResolvedValue([{ id: "o1" }] as never);
    vi.mocked(createPayment).mockResolvedValue({
      transaction: {} as never,
      paymentUrl: "http://pay",
    });
    const res = await checkoutAction("addr1", ["it1"], PaymentMethod.MOCK_MOMO);
    expect(createPayment).toHaveBeenCalledWith("o1", PaymentMethod.MOCK_MOMO);
    expect(res).toEqual({
      ok: true,
      data: { orderIds: ["o1"], paymentUrl: "http://pay" },
    });
  });

  it("still succeeds when payment initiation fails (falls back)", async () => {
    vi.mocked(createOrder).mockResolvedValue([{ id: "o1" }] as never);
    vi.mocked(createPayment).mockRejectedValue(new Error("pay down"));
    const res = await checkoutAction("addr1", ["it1"], PaymentMethod.MOCK_MOMO);
    expect(res).toEqual({
      ok: true,
      data: { orderIds: ["o1"], paymentUrl: undefined },
    });
  });

  it("returns an error shape when order creation fails", async () => {
    vi.mocked(createOrder).mockRejectedValue(new Error("no stock"));
    const res = await checkoutAction("addr1", ["it1"], PaymentMethod.COD);
    expect(res).toEqual({ ok: false, error: "no stock" });
  });
});

describe("updateOrderStatusAction", () => {
  it("succeeds and revalidates", async () => {
    vi.mocked(updateOrderStatus).mockResolvedValue({} as never);
    const res = await updateOrderStatusAction(
      "o1",
      OrderStatus.SHIPPED,
      "TRK1",
    );
    expect(updateOrderStatus).toHaveBeenCalledWith(
      "o1",
      OrderStatus.SHIPPED,
      "TRK1",
    );
    expect(res.ok).toBe(true);
  });

  it("returns error message on failure", async () => {
    vi.mocked(updateOrderStatus).mockRejectedValue(new Error("bad state"));
    const res = await updateOrderStatusAction("o1", OrderStatus.SHIPPED);
    expect(res).toEqual({ ok: false, message: "bad state" });
  });
});

describe("cancelOrderAction", () => {
  it("cancels with a reason", async () => {
    vi.mocked(cancelOrder).mockResolvedValue({} as never);
    const res = await cancelOrderAction("o1", "changed mind");
    expect(cancelOrder).toHaveBeenCalledWith("o1", "changed mind");
    expect(res.ok).toBe(true);
  });
});

describe("payment actions", () => {
  it("createPaymentAction returns the payment url", async () => {
    vi.mocked(createPayment).mockResolvedValue({
      transaction: {} as never,
      paymentUrl: "http://pay",
    });
    const res = await createPaymentAction("o1", PaymentMethod.MOCK_BANK);
    expect(res).toEqual({ ok: true, data: { paymentUrl: "http://pay" } });
  });

  it("processMockPaymentAction reflects the transaction success flag", async () => {
    vi.mocked(processMockPayment).mockResolvedValue({
      transaction: {} as never,
      success: true,
      message: "paid",
    });
    const res = await processMockPaymentAction("tx1", true);
    expect(res).toEqual({ ok: true, data: { message: "paid" } });
  });

  it("processMockPaymentAction returns error shape on throw", async () => {
    vi.mocked(processMockPayment).mockRejectedValue(new Error("gateway"));
    const res = await processMockPaymentAction("tx1", false);
    expect(res).toEqual({ ok: false, error: "gateway" });
  });

  it("processMockPaymentAction maps a failed simulation to an error", async () => {
    vi.mocked(processMockPayment).mockResolvedValue({
      transaction: {} as never,
      success: false,
      message: "card declined",
    });
    const res = await processMockPaymentAction("tx1", false);
    expect(res).toEqual({ ok: false, error: "card declined" });
  });
});
