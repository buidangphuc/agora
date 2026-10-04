import { revalidatePath } from "next/cache";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { OrderStatus } from "@/generated/platform/order/v1/order_pb.js";
import { PaymentMethod } from "@/generated/platform/payment/v1/payment_pb.js";
import { reorder } from "@/lib/gateway/cart";
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
  reorderAction,
  updateOrderStatusAction,
} from "./actions";

vi.mock("next/cache", () => ({ revalidatePath: vi.fn() }));
vi.mock("@/lib/gateway/cart", () => ({ reorder: vi.fn() }));
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
    expect(res).toMatchObject({
      ok: true,
      orderIds: ["o1"],
      paymentUrl: undefined,
      message: "Đặt hàng thành công!",
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
    expect(res.paymentUrl).toBe("http://pay");
  });

  it("still succeeds when payment initiation fails (falls back)", async () => {
    vi.mocked(createOrder).mockResolvedValue([{ id: "o1" }] as never);
    vi.mocked(createPayment).mockRejectedValue(new Error("pay down"));
    const res = await checkoutAction("addr1", ["it1"], PaymentMethod.MOCK_MOMO);
    expect(res.ok).toBe(true);
    expect(res.paymentUrl).toBeUndefined();
  });

  it("returns an error shape when order creation fails", async () => {
    vi.mocked(createOrder).mockRejectedValue(new Error("no stock"));
    const res = await checkoutAction("addr1", ["it1"], PaymentMethod.COD);
    expect(res).toEqual({ ok: false, message: "no stock" });
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
  it("cancels with a reason and revalidates the list and the detail", async () => {
    vi.mocked(cancelOrder).mockResolvedValue({} as never);
    const res = await cancelOrderAction("o1", "changed mind");
    expect(cancelOrder).toHaveBeenCalledWith("o1", "changed mind");
    expect(res).toEqual({ ok: true });
    expect(revalidatePath).toHaveBeenCalledWith("/account/orders");
    expect(revalidatePath).toHaveBeenCalledWith("/account/orders/o1");
  });

  it("returns the error shape without throwing or revalidating", async () => {
    vi.mocked(cancelOrder).mockRejectedValue(new Error("invalid status"));
    const res = await cancelOrderAction("o1");
    expect(res).toEqual({ ok: false, error: "invalid status" });
    expect(revalidatePath).not.toHaveBeenCalled();
  });
});

describe("reorderAction", () => {
  it("returns the item count as data and revalidates", async () => {
    vi.mocked(reorder).mockResolvedValue({ totalItems: 3 } as never);
    const res = await reorderAction("o1");
    expect(reorder).toHaveBeenCalledWith("o1");
    expect(res).toEqual({ ok: true, data: { totalItems: 3 } });
    expect(revalidatePath).toHaveBeenCalledWith("/cart");
    expect(revalidatePath).toHaveBeenCalledWith("/account/orders");
    expect(revalidatePath).toHaveBeenCalledWith("/account/orders/o1");
  });

  it("returns the error shape when the gateway throws", async () => {
    vi.mocked(reorder).mockRejectedValue(new Error("empty"));
    await expect(reorderAction("o1")).resolves.toEqual({
      ok: false,
      error: "empty",
    });
  });
});

describe("payment actions", () => {
  it("createPaymentAction returns the payment url", async () => {
    vi.mocked(createPayment).mockResolvedValue({
      transaction: {} as never,
      paymentUrl: "http://pay",
    });
    const res = await createPaymentAction("o1", PaymentMethod.MOCK_BANK);
    expect(res).toEqual({ ok: true, paymentUrl: "http://pay" });
  });

  it("processMockPaymentAction reflects the transaction success flag", async () => {
    vi.mocked(processMockPayment).mockResolvedValue({
      transaction: {} as never,
      success: true,
      message: "paid",
    });
    const res = await processMockPaymentAction("tx1", true);
    expect(res).toEqual({ ok: true, message: "paid" });
  });

  it("processMockPaymentAction returns error shape on throw", async () => {
    vi.mocked(processMockPayment).mockRejectedValue(new Error("gateway"));
    const res = await processMockPaymentAction("tx1", false);
    expect(res).toEqual({ ok: false, message: "gateway" });
  });
});
