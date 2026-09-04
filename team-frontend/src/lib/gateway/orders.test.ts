import { beforeEach, describe, expect, it, vi } from "vitest";

import { OrderStatus } from "@/generated/platform/order/v1/order_pb.js";
import { PaymentMethod } from "@/generated/platform/payment/v1/payment_pb.js";

import { makeClients } from "./client.js";
import {
  calculateShippingFee,
  cancelOrder,
  createOrder,
  getOrder,
  listBuyerOrders,
  updateOrderStatus,
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
    listSellerOrders: vi.fn(),
    updateOrderStatus: vi.fn(),
    cancelOrder: vi.fn(),
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
      voucherCode: "",
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

  it("createOrder applies defaults for missing args (COD, empty ids)", async () => {
    const order = stubOrder({
      createOrder: vi.fn().mockResolvedValue({ orders: [] }),
    });
    await createOrder();
    expect(order.createOrder).toHaveBeenCalledWith({
      addressId: "",
      itemIds: [],
      paymentMethod: PaymentMethod.COD,
      voucherCode: "",
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

  it("calculateShippingFee falls back to a standard fee on error", async () => {
    stubOrder({
      calculateShippingFee: vi.fn().mockRejectedValue(new Error("down")),
    });
    await expect(calculateShippingFee("HCM", 100000)).resolves.toEqual({
      shippingFee: 20000,
      isFreeShipping: false,
      message: "Phí vận chuyển tiêu chuẩn",
    });
  });

  it("updateOrderStatus throws when the gateway returns no order", async () => {
    stubOrder({ updateOrderStatus: vi.fn().mockResolvedValue({}) });
    await expect(
      updateOrderStatus("o1", OrderStatus.SHIPPED, "TRK9"),
    ).rejects.toThrow("update order status failed");
  });

  it("cancelOrder forwards id + reason and maps the order", async () => {
    const order = stubOrder({
      cancelOrder: vi.fn().mockResolvedValue({ order: protoOrder }),
    });
    const res = await cancelOrder("o1", "changed mind");
    expect(order.cancelOrder).toHaveBeenCalledWith({
      id: "o1",
      reason: "changed mind",
    });
    expect(res.id).toBe("o1");
  });
});
