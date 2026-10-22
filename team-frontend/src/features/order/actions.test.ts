import { Code, ConnectError } from "@connectrpc/connect";
import { revalidatePath } from "next/cache";
import { beforeEach, describe, expect, it, vi } from "vitest";

import {
  CancelReason,
  OrderStatus,
} from "@/generated/platform/order/v1/order_pb.js";
import { PaymentMethod } from "@/generated/platform/payment/v1/payment_pb.js";
import { reorder } from "@/lib/gateway/cart";
import {
  cancelOrder,
  confirmReceipt,
  createOrder,
  updateOrderStatus,
} from "@/lib/gateway/orders";
import { createPayment, processMockPayment } from "@/lib/gateway/payment";

import {
  cancelOrderAction,
  checkoutAction,
  confirmReceiptAction,
  createPaymentAction,
  processMockPaymentAction,
  reorderAction,
  updateOrderStatusAction,
} from "./actions";
import {
  ADDRESS_CHANGED_MESSAGE,
  ADDRESS_NOT_FOUND_MESSAGE,
  PRICE_CHANGED_MESSAGE,
  UNAVAILABLE_RETRY_MESSAGE,
} from "./shippingCopy";

vi.mock("next/cache", () => ({ revalidatePath: vi.fn() }));
vi.mock("@/lib/gateway/cart", () => ({ reorder: vi.fn() }));
vi.mock("@/lib/gateway/orders", () => ({
  createOrder: vi.fn(),
  updateOrderStatus: vi.fn(),
  cancelOrder: vi.fn(),
  confirmReceipt: vi.fn(),
}));
vi.mock("@/lib/gateway/payment", () => ({
  createPayment: vi.fn(),
  processMockPayment: vi.fn(),
}));

beforeEach(() => vi.clearAllMocks());

const ADDRESS = {
  recipientName: "Nguyễn An",
  phone: "0901234567",
  street: "12 Lê Lợi",
  ward: "Hải Châu 1",
  district: "Hải Châu",
  city: "Đà Nẵng",
};

describe("checkoutAction", () => {
  it("Checkout coherence: placing an online order goes to the first pay page - online checkout returns the first pay page without creating a payment", async () => {
    vi.mocked(createOrder).mockResolvedValue([
      { id: "o1" },
      { id: "o2" },
    ] as never);
    const res = await checkoutAction({
      addressId: "addr1",
      itemIds: ["it1"],
      paymentMethod: PaymentMethod.MOCK_MOMO,
    });
    expect(createPayment).toHaveBeenCalledTimes(0);
    expect(res).toEqual({
      ok: true,
      data: { orderIds: ["o1", "o2"], paymentUrl: "/checkout/pay/o1" },
    });
  });

  it("Checkout coherence: placing an online order goes to the first pay page - COD returns no pay page and no payment is created", async () => {
    vi.mocked(createOrder).mockResolvedValue([{ id: "o1" }] as never);
    const res = await checkoutAction({
      addressId: "addr1",
      itemIds: ["it1"],
      paymentMethod: PaymentMethod.COD,
    });
    expect(createPayment).not.toHaveBeenCalled();
    expect(res).toEqual({
      ok: true,
      data: { orderIds: ["o1"], paymentUrl: undefined },
    });
  });

  it("Checkout coherence: placing an online order goes to the first pay page - an online order never calls createPayment even when it would fail", async () => {
    vi.mocked(createOrder).mockResolvedValue([{ id: "o9" }] as never);
    vi.mocked(createPayment).mockRejectedValue(new Error("pay down"));
    const res = await checkoutAction({
      addressId: "addr1",
      paymentMethod: PaymentMethod.MOCK_BANK,
    });
    expect(createPayment).not.toHaveBeenCalled();
    expect(res).toEqual({
      ok: true,
      data: { orderIds: ["o9"], paymentUrl: "/checkout/pay/o9" },
    });
  });

  it("Checkout coherence: only valid vouchers are placed - scoped vouchers, items and confirmed address reach createOrder and no legacy voucher is sent", async () => {
    vi.mocked(createOrder).mockResolvedValue([{ id: "o1" }] as never);
    await checkoutAction({
      addressId: "addr1",
      itemIds: ["it1", "it2"],
      paymentMethod: PaymentMethod.COD,
      shopVouchers: [{ sellerId: "s1", code: "SHOPA" }],
      platformVoucherCode: "SAVE10",
      confirmedAddress: ADDRESS,
      idempotencyKey: "key-1",
    });
    expect(createOrder).toHaveBeenCalledTimes(1);
    expect(createOrder).toHaveBeenCalledWith(
      "addr1",
      ["it1", "it2"],
      PaymentMethod.COD,
      "key-1",
      {
        shopVouchers: [{ sellerId: "s1", code: "SHOPA" }],
        platformVoucherCode: "SAVE10",
        confirmedAddress: ADDRESS,
      },
    );
  });

  it("Checkout coherence: only valid vouchers are placed - no vouchers means empty slots", async () => {
    vi.mocked(createOrder).mockResolvedValue([{ id: "o1" }] as never);
    await checkoutAction({
      addressId: "addr1",
      paymentMethod: PaymentMethod.COD,
      idempotencyKey: "key-2",
    });
    const opts = vi.mocked(createOrder).mock.calls[0]?.[4];
    expect(opts?.shopVouchers ?? []).toEqual([]);
    expect(opts?.platformVoucherCode ?? "").toBe("");
    expect(vi.mocked(createOrder).mock.calls[0]?.[3]).toBe("key-2");
  });

  it("maps an aborted checkout to a retryable in-progress message", async () => {
    vi.mocked(createOrder).mockRejectedValue(
      new ConnectError("checkout already in progress", Code.Aborted),
    );
    const res = await checkoutAction({
      addressId: "addr1",
      paymentMethod: PaymentMethod.COD,
    });
    expect(res).toEqual({
      ok: false,
      error: "Đơn hàng đang được xử lý, vui lòng thử lại sau giây lát.",
    });
  });

  it("returns an error shape when order creation fails", async () => {
    vi.mocked(createOrder).mockRejectedValue(new Error("no stock"));
    const res = await checkoutAction({
      addressId: "addr1",
      paymentMethod: PaymentMethod.COD,
    });
    expect(res).toEqual({ ok: false, error: "no stock" });
  });
});

describe("checkoutAction error mapping", () => {
  const input = {
    addressId: "addr1",
    paymentMethod: PaymentMethod.COD,
    confirmedAddress: ADDRESS,
    idempotencyKey: "k",
  };

  it("Checkout coherence: an edited address is refused and refreshed - maps address changed", async () => {
    vi.mocked(createOrder).mockRejectedValue(
      new ConnectError("address changed", Code.FailedPrecondition),
    );
    const res = await checkoutAction(input);
    expect(ADDRESS_CHANGED_MESSAGE).toEqual(expect.any(String));
    expect(ADDRESS_CHANGED_MESSAGE.length).toBeGreaterThan(0);
    expect(res).toEqual({
      ok: false,
      error: ADDRESS_CHANGED_MESSAGE,
      addressChanged: true,
    });
    expect(revalidatePath).toHaveBeenCalledWith("/checkout");
    expect(createPayment).not.toHaveBeenCalled();
  });

  it("Checkout coherence: a missing address is explained - maps address not found and leaves other preconditions generic", async () => {
    vi.mocked(createOrder).mockRejectedValue(
      new ConnectError("address not found", Code.FailedPrecondition),
    );
    const res = await checkoutAction({ ...input, addressId: "" });
    expect(ADDRESS_NOT_FOUND_MESSAGE.length).toBeGreaterThan(0);
    expect(res).toEqual({
      ok: false,
      error: ADDRESS_NOT_FOUND_MESSAGE,
      addressNotFound: true,
    });
    expect(res).not.toHaveProperty("addressChanged");
    expect(res).not.toHaveProperty("priceChanged");

    const other = new ConnectError(
      "insufficient wallet balance",
      Code.FailedPrecondition,
    );
    vi.mocked(createOrder).mockRejectedValue(other);
    const generic = await checkoutAction(input);
    expect(generic).toEqual({ ok: false, error: other.message });
    expect(generic).not.toHaveProperty("addressNotFound");
    expect(generic).not.toHaveProperty("addressChanged");
  });

  it("Checkout coherence: a missing address is explained - address changed and not found are distinct messages", () => {
    expect(ADDRESS_CHANGED_MESSAGE).not.toBe(ADDRESS_NOT_FOUND_MESSAGE);
  });

  it("Checkout coherence: an identity outage asks the buyer to retry - maps unavailable to a retry message", async () => {
    vi.mocked(createOrder).mockRejectedValue(
      new ConnectError("identity down", Code.Unavailable),
    );
    const res = await checkoutAction(input);
    expect(UNAVAILABLE_RETRY_MESSAGE.length).toBeGreaterThan(0);
    expect(res).toEqual({ ok: false, error: UNAVAILABLE_RETRY_MESSAGE });
    expect(res).not.toHaveProperty("priceChanged");
    expect(res).not.toHaveProperty("addressChanged");
    expect(revalidatePath).not.toHaveBeenCalledWith("/cart");
  });
});

describe("checkoutAction price change", () => {
  it("maps price changed to a refresh-worthy failure", async () => {
    vi.mocked(createOrder).mockRejectedValue(
      new ConnectError("price changed", Code.FailedPrecondition),
    );
    const res = await checkoutAction({
      addressId: "addr1",
      paymentMethod: PaymentMethod.COD,
    });
    expect(res).toEqual({
      ok: false,
      error: PRICE_CHANGED_MESSAGE,
      priceChanged: true,
    });
    expect(revalidatePath).toHaveBeenCalledWith("/cart");
    expect(revalidatePath).toHaveBeenCalledWith("/checkout");
    expect(createPayment).not.toHaveBeenCalled();
  });
});

describe("updateOrderStatusAction", () => {
  it("returns ok with the new state and revalidates list and detail", async () => {
    vi.mocked(updateOrderStatus).mockResolvedValue({
      status: OrderStatus.SHIPPED,
      trackingNumber: "TRK1",
    } as never);
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
    expect(res).toEqual({
      ok: true,
      data: { status: OrderStatus.SHIPPED, trackingNumber: "TRK1" },
    });
    expect(revalidatePath).toHaveBeenCalledWith("/seller/orders");
    expect(revalidatePath).toHaveBeenCalledWith("/seller/orders/o1");
  });

  it("returns { ok: false, error } on failure and revalidates nothing", async () => {
    vi.mocked(updateOrderStatus).mockRejectedValue(new Error("bad state"));
    const res = await updateOrderStatusAction("o1", OrderStatus.SHIPPED);
    expect(res).toEqual({ ok: false, error: "bad state" });
    expect(revalidatePath).not.toHaveBeenCalled();
  });
});

describe("cancelOrderAction", () => {
  it("cancel sends the reason code (Order lifecycle: cancel sends the reason code with the label)", async () => {
    vi.mocked(cancelOrder).mockResolvedValue({} as never);
    const res = await cancelOrderAction("o1", "wrong_item");
    expect(cancelOrder).toHaveBeenCalledTimes(1);
    expect(cancelOrder).toHaveBeenCalledWith(
      "o1",
      "Shop tư vấn hoặc giao sai phân loại",
      CancelReason.WRONG_ITEM,
    );
    expect(res).toEqual({ ok: true });
    expect(revalidatePath).toHaveBeenCalledWith("/account/orders");
    expect(revalidatePath).toHaveBeenCalledWith("/account/orders/o1");
  });

  it.each([
    [
      "changed_mind",
      "Tôi muốn thay đổi địa chỉ hoặc đổi ý",
      CancelReason.CHANGED_MIND,
    ],
    ["damaged", "Sản phẩm bị lỗi / hư hỏng", CancelReason.DAMAGED],
    [
      "delivery_delay",
      "Thời gian giao hàng quá lâu",
      CancelReason.DELIVERY_DELAY,
    ],
  ])(
    "cancel sends the reason code for %s (Order lifecycle: cancel sends the reason code with the label)",
    async (value, label, code) => {
      vi.mocked(cancelOrder).mockResolvedValue({} as never);
      await cancelOrderAction("o1", value);
      expect(cancelOrder).toHaveBeenCalledWith("o1", label, code);
    },
  );

  it.each([
    "payment_expired",
    "admin_force_fail",
    "",
    "CHANGED_MIND",
    "Tôi muốn thay đổi địa chỉ hoặc đổi ý",
    "toString",
    "__proto__",
  ])(
    "unknown cancel reason %j is refused before any call (Order lifecycle: an unknown cancel reason is refused before any call)",
    async (value) => {
      const res = await cancelOrderAction("o1", value);
      expect(res).toEqual({ ok: false, error: "Lý do hủy không hợp lệ." });
      expect(cancelOrder).not.toHaveBeenCalled();
      expect(revalidatePath).not.toHaveBeenCalled();
    },
  );

  it("a missing reason is refused before any call (Order lifecycle: an unknown cancel reason is refused before any call)", async () => {
    const res = await cancelOrderAction("o1");
    expect(res).toEqual({ ok: false, error: "Lý do hủy không hợp lệ." });
    expect(cancelOrder).not.toHaveBeenCalled();
  });

  it("cancel of a foreign order is refused (Order lifecycle: cancelling an order that is not the buyer's is refused)", async () => {
    vi.mocked(cancelOrder).mockRejectedValue(
      new ConnectError("order not found", Code.PermissionDenied),
    );
    const res = await cancelOrderAction("o2", "changed_mind");
    expect(res.ok).toBe(false);
    if (!res.ok) expect(res.error).toContain("order not found");
    expect(revalidatePath).not.toHaveBeenCalled();
  });

  it("returns the error shape without throwing or revalidating (Order lifecycle: cancelling an order that is not the buyer's is refused)", async () => {
    vi.mocked(cancelOrder).mockRejectedValue(new Error("invalid status"));
    const res = await cancelOrderAction("o1", "changed_mind");
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

describe("confirmReceiptAction", () => {
  it("Order lifecycle: confirming receipt calls the server and completes the order - action calls confirmReceipt and revalidates list and detail", async () => {
    vi.mocked(confirmReceipt).mockResolvedValue({ id: "o1" } as never);
    const res = await confirmReceiptAction("o1");
    expect(res.ok).toBe(true);
    expect(confirmReceipt).toHaveBeenCalledTimes(1);
    expect(confirmReceipt).toHaveBeenCalledWith("o1");
    expect(revalidatePath).toHaveBeenCalledWith("/account/orders");
    expect(revalidatePath).toHaveBeenCalledWith("/account/orders/o1");
  });

  it("Order lifecycle: a confirm that lost to auto-complete is explained - confirm after auto-complete is explained", async () => {
    vi.mocked(confirmReceipt).mockRejectedValue(
      new ConnectError("order is not shipped", Code.FailedPrecondition),
    );
    const res = await confirmReceiptAction("o1");
    expect(res).toEqual({ ok: false, error: "Đơn hàng đã được hoàn thành." });
    expect(revalidatePath).toHaveBeenCalledWith("/account/orders");
    expect(revalidatePath).toHaveBeenCalledWith("/account/orders/o1");
  });

  it("Order lifecycle: a refused confirm shows the server error - refused confirm returns the server error", async () => {
    vi.mocked(confirmReceipt).mockRejectedValue(new Error("forbidden"));
    const res = await confirmReceiptAction("o1");
    expect(res).toEqual({ ok: false, error: "forbidden" });
    expect(revalidatePath).not.toHaveBeenCalled();
  });

  it("Order lifecycle: a refused confirm shows the server error - permission denied is not mapped to completed", async () => {
    vi.mocked(confirmReceipt).mockRejectedValue(
      new ConnectError("not your order", Code.PermissionDenied),
    );
    const res = await confirmReceiptAction("o1");
    expect(res.ok).toBe(false);
    expect(res.ok === false && res.error).not.toBe(
      "Đơn hàng đã được hoàn thành.",
    );
    expect(revalidatePath).not.toHaveBeenCalled();
  });
});
