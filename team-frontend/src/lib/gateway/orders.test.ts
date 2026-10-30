import { Code, ConnectError } from "@connectrpc/connect";
import { beforeEach, describe, expect, it, vi } from "vitest";

import {
  CancelReason,
  CheckoutVoucherScope,
  OrderStatus,
  ReturnStatus,
} from "@/generated/platform/order/v1/order_pb.js";
import { PaymentMethod } from "@/generated/platform/payment/v1/payment_pb.js";

import { makeClients } from "./client.js";
import {
  calculateShippingFee,
  cancelOrder,
  confirmReceipt,
  createOrder,
  getOrder,
  getOrderResult,
  listBuyerOrders,
  listBuyerOrdersResult,
  listOrderReturns,
  previewCheckout,
} from "./orders.js";

vi.mock("./client.js", () => ({ makeClients: vi.fn() }));
vi.mock("./session.js", () => ({
  getToken: vi.fn(() => "test-token"),
  SESSION_COOKIE: "session",
}));

function stubOrder(rpcs: Record<string, ReturnType<typeof vi.fn>>) {
  const order = {
    calculateShippingFee: vi.fn(),
    createOrder: vi.fn(),
    getOrder: vi.fn(),
    listBuyerOrders: vi.fn(),
    cancelOrder: vi.fn(),
    confirmReceipt: vi.fn(),
    previewCheckout: vi.fn(),
    ...rpcs,
  };
  vi.mocked(makeClients).mockReturnValue({ order } as never);
  return order;
}

const protoOrder = {
  id: "o1",
  buyerId: "b1",
  sellerId: "s1",
  status: OrderStatus.PENDING,
  totalAmount: 120000n,
  itemsSubtotal: 100000n,
  shippingFee: 20000n,
  paymentMethod: PaymentMethod.COD,
  currency: "VND",
  trackingNumber: "TRK1",
  shippingAddress: {
    recipientName: "A",
    phone: "090",
    street: "1 Main",
    ward: "W",
    district: "D",
    city: "HCM",
  },
  items: [
    {
      id: "it1",
      listingId: "l1",
      variantId: "v1",
      title: "Item",
      variantName: "Red",
      quantity: 2,
      unitPrice: 50000n,
      imageUrl: "img.png",
    },
  ],
  createdAt: { seconds: 1_700_000_000 },
};

beforeEach(() => vi.clearAllMocks());

describe("orders gateway wrapper", () => {
  it("createOrder maps args and view-maps the returned orders", async () => {
    const order = stubOrder({
      createOrder: vi.fn().mockResolvedValue({ orders: [protoOrder] }),
    });
    const res = await createOrder("addr1", ["it1"], PaymentMethod.MOCK_MOMO);
    expect(order.createOrder).toHaveBeenCalledWith({
      addressId: "addr1",
      itemIds: ["it1"],
      paymentMethod: PaymentMethod.MOCK_MOMO,
      shopVouchers: [],
      platformVoucherCode: "",
    });
    expect(res[0]).toMatchObject({
      id: "o1",
      status: OrderStatus.PENDING,
      statusText: "Chờ xử lý",
      totalAmount: 120000,
      itemsSubtotal: 100000,
      shippingFee: 20000,
      addressFull: "1 Main, W, D, HCM",
      recipientName: "A",
    });
    expect(res[0].items[0]).toMatchObject({ id: "it1", unitPrice: 50000 });
  });

  it("createOrder sends the Idempotency-Key call header when a key is given", async () => {
    const order = stubOrder({
      createOrder: vi.fn().mockResolvedValue({ orders: [] }),
    });
    await createOrder("a", [], PaymentMethod.COD, "key-1");
    expect(order.createOrder).toHaveBeenCalledWith(
      {
        addressId: "a",
        itemIds: [],
        paymentMethod: PaymentMethod.COD,
        shopVouchers: [],
        platformVoucherCode: "",
      },
      { headers: { "Idempotency-Key": "key-1" } },
    );
  });

  it("createOrder applies defaults for missing args (COD, empty ids)", async () => {
    const order = stubOrder({
      createOrder: vi.fn().mockResolvedValue({ orders: [] }),
    });
    await createOrder();
    expect(order.createOrder).toHaveBeenCalledWith({
      addressId: "",
      itemIds: [],
      paymentMethod: PaymentMethod.COD,
      shopVouchers: [],
      platformVoucherCode: "",
    });
  });

  it("getOrder returns null when the RPC fails", async () => {
    stubOrder({ getOrder: vi.fn().mockRejectedValue(new Error("nope")) });
    await expect(getOrder("o1")).resolves.toBeNull();
  });

  it("getOrder returns null when the order is absent", async () => {
    stubOrder({ getOrder: vi.fn().mockResolvedValue({ order: undefined }) });
    await expect(getOrder("o1")).resolves.toBeNull();
  });

  it("getOrderResult returns ok with the mapped order", async () => {
    stubOrder({ getOrder: vi.fn().mockResolvedValue({ order: protoOrder }) });
    const res = await getOrderResult("o1");
    expect(res.kind).toBe("ok");
    if (res.kind === "ok") expect(res.order.id).toBe("o1");
  });

  it("getOrderResult maps PermissionDenied to forbidden", async () => {
    stubOrder({
      getOrder: vi
        .fn()
        .mockRejectedValue(new ConnectError("no", Code.PermissionDenied)),
    });
    await expect(getOrderResult("o1")).resolves.toEqual({ kind: "forbidden" });
  });

  it("getOrderResult maps NotFound and an empty response to not_found", async () => {
    stubOrder({
      getOrder: vi
        .fn()
        .mockRejectedValueOnce(new ConnectError("gone", Code.NotFound))
        .mockResolvedValueOnce({ order: undefined }),
    });
    await expect(getOrderResult("o1")).resolves.toEqual({ kind: "not_found" });
    await expect(getOrderResult("o1")).resolves.toEqual({ kind: "not_found" });
  });

  it("getOrderResult maps any other failure to error", async () => {
    stubOrder({
      getOrder: vi
        .fn()
        .mockRejectedValueOnce(new ConnectError("down", Code.Unavailable))
        .mockRejectedValueOnce(new Error("boom")),
    });
    await expect(getOrderResult("o1")).resolves.toEqual({ kind: "error" });
    await expect(getOrderResult("o1")).resolves.toEqual({ kind: "error" });
  });

  it("listBuyerOrdersResult distinguishes a failed load from no orders", async () => {
    stubOrder({
      listBuyerOrders: vi
        .fn()
        .mockResolvedValueOnce({ orders: [] })
        .mockRejectedValueOnce(new Error("down")),
    });
    await expect(listBuyerOrdersResult()).resolves.toEqual({
      ok: true,
      orders: [],
    });
    await expect(listBuyerOrdersResult()).resolves.toEqual({ ok: false });
  });

  it("listBuyerOrders defaults the status filter and maps results", async () => {
    const order = stubOrder({
      listBuyerOrders: vi.fn().mockResolvedValue({ orders: [protoOrder] }),
    });
    const res = await listBuyerOrders();
    expect(order.listBuyerOrders).toHaveBeenCalledWith({
      statusFilter: OrderStatus.UNSPECIFIED,
    });
    expect(res).toHaveLength(1);
  });

  it("calculateShippingFee returns null on error", async () => {
    const order = stubOrder({
      calculateShippingFee: vi.fn().mockRejectedValue(new Error("down")),
    });
    await expect(calculateShippingFee("HCM", 100000)).resolves.toBeNull();
    // bounded by the 2000 ms call timeout
    expect(order.calculateShippingFee).toHaveBeenCalledWith(
      { city: "HCM", itemsSubtotal: 100000n },
      { timeoutMs: 2000 },
    );
  });

  it("calculateShippingFee returns the server quote on success", async () => {
    stubOrder({
      calculateShippingFee: vi.fn().mockResolvedValue({
        shippingFee: 20000n,
        isFreeShipping: false,
        message: "",
      }),
    });
    await expect(calculateShippingFee("Hà Nội", 350000)).resolves.toMatchObject(
      { shippingFee: 20000, isFreeShipping: false },
    );
  });

  it("Order lifecycle: cancel sends the reason code with the label", async () => {
    const order = stubOrder({
      cancelOrder: vi.fn().mockResolvedValue({ order: protoOrder }),
    });
    const res = await cancelOrder(
      "o1",
      "Đặt nhầm sản phẩm",
      CancelReason.WRONG_ITEM,
    );
    expect(order.cancelOrder).toHaveBeenCalledTimes(1);
    expect(order.cancelOrder).toHaveBeenCalledWith({
      id: "o1",
      reason: "Đặt nhầm sản phẩm",
      reasonCode: CancelReason.WRONG_ITEM,
    });
    expect(res.id).toBe("o1");
  });

  it("Order lifecycle: cancel sends reasonCode UNSPECIFIED when none is given", async () => {
    const order = stubOrder({
      cancelOrder: vi.fn().mockResolvedValue({ order: protoOrder }),
    });
    await cancelOrder("o1", "changed mind");
    expect(order.cancelOrder).toHaveBeenCalledWith({
      id: "o1",
      reason: "changed mind",
      reasonCode: CancelReason.UNSPECIFIED,
    });
  });

  it("Order lifecycle: confirming receipt calls the server and completes the order", async () => {
    const order = stubOrder({
      confirmReceipt: vi.fn().mockResolvedValue({
        order: { ...protoOrder, status: OrderStatus.COMPLETED },
      }),
    });
    const res = await confirmReceipt("o1");
    expect(order.confirmReceipt).toHaveBeenCalledTimes(1);
    expect(order.confirmReceipt).toHaveBeenCalledWith({ id: "o1" });
    expect(res.id).toBe("o1");
    expect(res.status).toBe(OrderStatus.COMPLETED);
  });

  it("Order lifecycle: confirmReceipt throws when the gateway returns no order", async () => {
    stubOrder({ confirmReceipt: vi.fn().mockResolvedValue({}) });
    await expect(confirmReceipt("o1")).rejects.toThrow();
  });

  it("Order lifecycle: confirmReceipt propagates a server refusal", async () => {
    stubOrder({
      confirmReceipt: vi
        .fn()
        .mockRejectedValue(
          new ConnectError("not shipped", Code.FailedPrecondition),
        ),
    });
    await expect(confirmReceipt("o1")).rejects.toBeInstanceOf(ConnectError);
  });

  it("Order lifecycle: ViewOrder deadlines are ISO strings, empty when unset", async () => {
    const ts = (sec: number) => ({ seconds: BigInt(sec), nanos: 0 });
    stubOrder({
      getOrder: vi
        .fn()
        .mockResolvedValueOnce({ order: protoOrder })
        .mockResolvedValueOnce({
          order: {
            ...protoOrder,
            paymentDueAt: ts(1_700_000_000),
            shipBy: ts(1_700_000_100),
            shippedAt: ts(1_700_000_200),
            completedAt: ts(1_700_000_300),
            returnWindowEndsAt: ts(1_700_000_400),
            autoCompleteAt: ts(1_700_000_500),
            cancelReason: CancelReason.PAYMENT_EXPIRED,
          },
        }),
    });
    const bare = await getOrder("o1");
    for (const k of [
      "paymentDueAt",
      "shipBy",
      "shippedAt",
      "completedAt",
      "returnWindowEndsAt",
      "autoCompleteAt",
    ] as const) {
      expect(bare?.[k]).toBe("");
    }
    expect(bare?.cancelReason).toBe(CancelReason.UNSPECIFIED);

    const full = await getOrder("o1");
    expect(full?.paymentDueAt).toBe(new Date(1_700_000_000_000).toISOString());
    expect(full?.shipBy).toBe(new Date(1_700_000_100_000).toISOString());
    expect(full?.shippedAt).toBe(new Date(1_700_000_200_000).toISOString());
    expect(full?.completedAt).toBe(new Date(1_700_000_300_000).toISOString());
    expect(full?.returnWindowEndsAt).toBe(
      new Date(1_700_000_400_000).toISOString(),
    );
    expect(full?.autoCompleteAt).toBe(
      new Date(1_700_000_500_000).toISOString(),
    );
    expect(full?.cancelReason).toBe(CancelReason.PAYMENT_EXPIRED);
  });

  it("getOrder maps paid_at (empty when never paid online)", async () => {
    stubOrder({
      getOrder: vi
        .fn()
        .mockResolvedValueOnce({ order: protoOrder })
        .mockResolvedValueOnce({
          order: { ...protoOrder, paidAt: { seconds: 1_700_000_000 } },
        }),
    });
    expect((await getOrder("o1"))?.paidAt).toBe("");
    expect((await getOrder("o1"))?.paidAt).not.toBe("");
  });

  it("listOrderReturns maps the returns through the gateway", async () => {
    const listOrderReturnsRpc = vi.fn().mockResolvedValue({
      returns: [
        {
          id: "r1",
          orderId: "o1",
          reason: "hỏng",
          refundAmount: 200000n,
          status: ReturnStatus.REFUNDED,
          canEscalate: false,
        },
      ],
    });
    stubOrder({ listOrderReturns: listOrderReturnsRpc });
    const res = await listOrderReturns("o1");
    expect(listOrderReturnsRpc).toHaveBeenCalledWith({ orderId: "o1" });
    expect(res).toEqual([
      {
        id: "r1",
        orderId: "o1",
        reason: "hỏng",
        refundAmount: 200000,
        status: ReturnStatus.REFUNDED,
        statusText: "Đã hoàn tiền",
        canEscalate: false,
      },
    ]);
  });

  it("Checkout coherence: two shops show two groups with their own shipping", async () => {
    const order = stubOrder({
      previewCheckout: vi.fn().mockResolvedValue({
        groups: [
          {
            sellerId: "s1",
            itemIds: ["i1"],
            itemsSubtotal: 100000n,
            shippingFee: 20000n,
            isFreeShipping: false,
            shopVoucherCode: "SHOP1",
            shopDiscountAmount: 5000n,
            platformDiscountAmount: 3000n,
            totalAmount: 112000n,
            estimatedDeliveryAt: { seconds: 1_700_000_000n },
          },
          {
            sellerId: "s2",
            itemIds: ["i2", "i3"],
            itemsSubtotal: 600000n,
            shippingFee: 0n,
            isFreeShipping: true,
            totalAmount: 597000n,
          },
        ],
        vouchers: [
          {
            code: "SHOP1",
            scope: CheckoutVoucherScope.SHOP,
            sellerId: "s1",
            valid: true,
            reason: "",
            discountAmount: 5000n,
          },
          {
            code: "SAVE10",
            scope: CheckoutVoucherScope.PLATFORM,
            sellerId: "",
            valid: false,
            reason: "min spend",
            discountAmount: 0n,
          },
        ],
        itemsSubtotal: 700000n,
        shippingFee: 20000n,
        discountAmount: 8000n,
        totalAmount: 709000n,
        shippingQuoted: true,
        shippingAddress: {
          id: "a1",
          recipientName: "A",
          phone: "090",
          street: "1 Main",
          ward: "W",
          district: "D",
          city: "HCM",
        },
      }),
    });
    const res = await previewCheckout({
      addressId: "a1",
      itemIds: ["i1", "i2", "i3"],
      shopVouchers: [{ sellerId: "s1", code: "SHOP1" }],
      platformVoucherCode: "SAVE10",
    });
    expect(order.previewCheckout).toHaveBeenCalledWith(
      {
        addressId: "a1",
        itemIds: ["i1", "i2", "i3"],
        shopVouchers: [{ sellerId: "s1", code: "SHOP1" }],
        platformVoucherCode: "SAVE10",
      },
      { timeoutMs: 5000 },
    );
    expect(res.groups).toHaveLength(2);
    expect(res.groups[0]).toMatchObject({
      sellerId: "s1",
      itemIds: ["i1"],
      itemsSubtotal: 100000,
      shippingFee: 20000,
      isFreeShipping: false,
      shopVoucherCode: "SHOP1",
      shopDiscountAmount: 5000,
      platformDiscountAmount: 3000,
      totalAmount: 112000,
      estimatedDeliveryAt: "2023-11-14T22:13:20.000Z",
    });
    expect(res.groups[1]).toMatchObject({
      sellerId: "s2",
      shippingFee: 0,
      isFreeShipping: true,
      totalAmount: 597000,
      estimatedDeliveryAt: null,
    });
    expect(res.vouchers).toEqual([
      {
        code: "SHOP1",
        scope: CheckoutVoucherScope.SHOP,
        sellerId: "s1",
        valid: true,
        reason: "",
        discountAmount: 5000,
      },
      {
        code: "SAVE10",
        scope: CheckoutVoucherScope.PLATFORM,
        sellerId: "",
        valid: false,
        reason: "min spend",
        discountAmount: 0,
      },
    ]);
    expect(res).toMatchObject({
      itemsSubtotal: 700000,
      shippingFee: 20000,
      discountAmount: 8000,
      totalAmount: 709000,
      shippingQuoted: true,
      shippingAddress: {
        recipientName: "A",
        phone: "090",
        street: "1 Main",
        ward: "W",
        district: "D",
        city: "HCM",
      },
    });
  });

  it("Checkout coherence: previewCheckout reports shippingQuoted false and no address when absent", async () => {
    stubOrder({
      previewCheckout: vi.fn().mockResolvedValue({
        groups: [],
        vouchers: [],
        itemsSubtotal: 0n,
        shippingFee: 0n,
        discountAmount: 0n,
        totalAmount: 0n,
        shippingQuoted: false,
      }),
    });
    const res = await previewCheckout({ itemIds: [] });
    expect(res.shippingQuoted).toBe(false);
    expect(res.shippingAddress).toBeNull();
    expect(res.groups).toEqual([]);
  });

  it("Checkout coherence: a failed preview shows no total", async () => {
    stubOrder({
      previewCheckout: vi
        .fn()
        .mockRejectedValue(new ConnectError("down", Code.Unavailable)),
    });
    const err = await previewCheckout({ itemIds: ["i1"] }).then(
      () => null,
      (e: unknown) => e,
    );
    expect(err).toBeInstanceOf(ConnectError);
    expect((err as ConnectError).code).toBe(Code.Unavailable);
  });

  it("Checkout coherence: the confirmed address is sent with the order", async () => {
    const order = stubOrder({
      createOrder: vi.fn().mockResolvedValue({ orders: [protoOrder] }),
    });
    await createOrder("a1", ["i1", "i2"], PaymentMethod.COD, "key-9", {
      shopVouchers: [{ sellerId: "s1", code: "SHOP1" }],
      platformVoucherCode: "SAVE10",
      confirmedAddress: {
        recipientName: "A",
        phone: "090",
        street: "1 Main",
        ward: "W",
        district: "D",
        city: "HCM",
      },
    });
    const [req, opts] = order.createOrder.mock.calls[0];
    expect(req).toMatchObject({
      addressId: "a1",
      itemIds: ["i1", "i2"],
      paymentMethod: PaymentMethod.COD,
      shopVouchers: [{ sellerId: "s1", code: "SHOP1" }],
      platformVoucherCode: "SAVE10",
      confirmedAddress: {
        recipientName: "A",
        phone: "090",
        street: "1 Main",
        ward: "W",
        district: "D",
        city: "HCM",
      },
    });
    expect(opts).toEqual({ headers: { "Idempotency-Key": "key-9" } });
  });

  it("Checkout coherence: createOrder no longer sends the legacy voucher_code", async () => {
    const order = stubOrder({
      createOrder: vi.fn().mockResolvedValue({ orders: [] }),
    });
    await createOrder("a1", [], PaymentMethod.COD, undefined, {
      platformVoucherCode: "SAVE10",
    });
    const [req] = order.createOrder.mock.calls[0];
    expect(req).not.toHaveProperty("voucherCode");
    expect(req.platformVoucherCode).toBe("SAVE10");
    expect(req.shopVouchers).toEqual([]);
  });

  it("Checkout coherence: ViewOrder carries checkout id and platform voucher fields", async () => {
    stubOrder({
      getOrder: vi.fn().mockResolvedValue({
        order: {
          ...protoOrder,
          checkoutId: "co1",
          platformVoucherCode: "SAVE10",
          platformDiscountAmount: 7000n,
        },
      }),
    });
    await expect(getOrder("o1")).resolves.toMatchObject({
      checkoutId: "co1",
      platformVoucherCode: "SAVE10",
      platformDiscountAmount: 7000,
    });
  });

  it("Checkout coherence: calculateShippingFee returns the delivery estimate as ISO or null", async () => {
    stubOrder({
      calculateShippingFee: vi
        .fn()
        .mockResolvedValueOnce({
          shippingFee: 20000n,
          isFreeShipping: false,
          message: "",
          estimatedDeliveryAt: { seconds: 1_700_000_000n },
        })
        .mockResolvedValueOnce({
          shippingFee: 20000n,
          isFreeShipping: false,
          message: "",
        }),
    });
    await expect(calculateShippingFee("HCM", 1)).resolves.toMatchObject({
      estimatedDeliveryAt: "2023-11-14T22:13:20.000Z",
    });
    await expect(calculateShippingFee("HCM", 1)).resolves.toMatchObject({
      estimatedDeliveryAt: null,
    });
  });

  it("Button is shown on an escalable return: canEscalate maps true", async () => {
    stubOrder({
      listOrderReturns: vi.fn().mockResolvedValue({
        returns: [
          {
            id: "r2",
            orderId: "o1",
            reason: "lỗi",
            refundAmount: 1000n,
            status: ReturnStatus.REJECTED,
            canEscalate: true,
          },
        ],
      }),
    });
    const res = await listOrderReturns("o1");
    expect(res[0]?.canEscalate).toBe(true);
  });

  it("Button is hidden when the return cannot be escalated: canEscalate maps false", async () => {
    stubOrder({
      listOrderReturns: vi.fn().mockResolvedValue({
        returns: [
          {
            id: "r3",
            orderId: "o1",
            reason: "lỗi",
            refundAmount: 1000n,
            status: ReturnStatus.PENDING,
            canEscalate: false,
          },
        ],
      }),
    });
    const res = await listOrderReturns("o1");
    expect(res[0]?.canEscalate).toBe(false);
  });
});
