import { Code, ConnectError } from "@connectrpc/connect";
import { beforeEach, describe, expect, it, vi } from "vitest";

import {
  PaymentMethod,
  PaymentRefundSource,
  PaymentStatus,
} from "@/generated/platform/payment/v1/payment_pb.js";

import { makeClients } from "./client.js";
import {
  createPayment,
  getOrderSettlement,
  getPayment,
  getPaymentMethodText,
  getPaymentStatusText,
  processMockPayment,
} from "./payment.js";

vi.mock("./client.js", () => ({ makeClients: vi.fn() }));
vi.mock("./session.js", () => ({
  getToken: vi.fn(() => "test-token"),
  SESSION_COOKIE: "session",
}));

function stubPayment(rpcs: Record<string, ReturnType<typeof vi.fn>>) {
  const payment = {
    createPayment: vi.fn(),
    getOrderSettlement: vi.fn(),
    getPayment: vi.fn(),
    processMockPayment: vi.fn(),
    ...rpcs,
  };
  vi.mocked(makeClients).mockReturnValue({ payment } as never);
  return payment;
}

const protoTx = {
  id: "tx1",
  orderId: "o1",
  buyerId: "b1",
  amount: 120000n,
  currency: "VND",
  method: PaymentMethod.MOCK_MOMO,
  status: PaymentStatus.PENDING,
  providerReference: "ref1",
  createdAt: { seconds: 1_700_000_000 },
  refundedAmount: 0n,
  refunds: [],
};

beforeEach(() => vi.clearAllMocks());

describe("payment label helpers", () => {
  it("Order lifecycle: CANCELLED payment status reads Đã hủy", () => {
    expect(getPaymentStatusText(PaymentStatus.CANCELLED)).toBe("Đã hủy");
  });

  it("maps method + status enums to Vietnamese labels", () => {
    expect(getPaymentMethodText(PaymentMethod.COD)).toContain("COD");
    expect(getPaymentStatusText(PaymentStatus.PAID)).toBe("Đã thanh toán");
    expect(getPaymentStatusText(PaymentStatus.PARTIALLY_REFUNDED)).toBe(
      "Đã hoàn một phần",
    );
  });
});

describe("payment gateway wrapper", () => {
  it("createPayment maps request and returns tx + paymentUrl", async () => {
    const payment = stubPayment({
      createPayment: vi
        .fn()
        .mockResolvedValue({ transaction: protoTx, paymentUrl: "http://pay" }),
    });
    const res = await createPayment("o1", PaymentMethod.MOCK_MOMO);
    expect(payment.createPayment).toHaveBeenCalledWith({
      orderId: "o1",
      method: PaymentMethod.MOCK_MOMO,
    });
    expect(res.paymentUrl).toBe("http://pay");
    expect(res.transaction).toMatchObject({
      id: "tx1",
      amount: 120000,
      methodText: "Ví điện tử MoMo (Demo)",
      statusText: "Chờ thanh toán",
    });
  });

  it("createPayment throws when no transaction is returned", async () => {
    stubPayment({ createPayment: vi.fn().mockResolvedValue({}) });
    await expect(createPayment("o1", PaymentMethod.COD)).rejects.toThrow(
      "create payment failed",
    );
  });

  it("getPayment maps the refunded amount and the refunds", async () => {
    stubPayment({
      getPayment: vi.fn().mockResolvedValue({
        transaction: {
          ...protoTx,
          status: PaymentStatus.PARTIALLY_REFUNDED,
          refundedAmount: 100000n,
          refunds: [
            {
              id: "f1",
              source: PaymentRefundSource.RETURN,
              sourceId: "r1",
              requestedAmount: 300000n,
              amount: 100000n,
              reason: "return_refunded",
            },
          ],
        },
      }),
    });
    const tx = await getPayment(undefined, "o1");
    expect(tx).toMatchObject({
      refundedAmount: 100000,
      statusText: "Đã hoàn một phần",
    });
    expect(tx?.refunds).toEqual([
      {
        id: "f1",
        source: PaymentRefundSource.RETURN,
        sourceId: "r1",
        requestedAmount: 300000,
        amount: 100000,
        reason: "return_refunded",
      },
    ]);
  });

  it("getPayment returns null on error", async () => {
    stubPayment({ getPayment: vi.fn().mockRejectedValue(new Error("x")) });
    await expect(getPayment("tx1")).resolves.toBeNull();
  });

  it("processMockPayment forwards flags and returns success + message", async () => {
    const payment = stubPayment({
      processMockPayment: vi.fn().mockResolvedValue({
        transaction: protoTx,
        success: true,
        message: "ok",
      }),
    });
    const res = await processMockPayment("tx1", true);
    expect(payment.processMockPayment).toHaveBeenCalledWith({
      transactionId: "tx1",
      simulateSuccess: true,
    });
    expect(res).toMatchObject({ success: true, message: "ok" });
  });
});

describe("getOrderSettlement", () => {
  const resp = {
    orderId: "o1",
    credited: 310000n,
    commission: 14000n,
    commissionReversed: 0n,
    refundDeducted: 0n,
    net: 296000n,
    commissionRateBps: 500,
  };

  it("Breakdown of a settled order: maps amounts and rate bps to percent, 2 s timeout", async () => {
    const payment = stubPayment({
      getOrderSettlement: vi.fn().mockResolvedValue(resp),
    });
    const res = await getOrderSettlement("o1");
    expect(payment.getOrderSettlement).toHaveBeenCalledWith(
      { orderId: "o1" },
      { timeoutMs: 2000 },
    );
    expect(res).toEqual({
      credited: 310000,
      commission: 14000,
      commissionRatePercent: 5,
      commissionReversed: 0,
      refundDeducted: 0,
      net: 296000,
    });
  });

  it("Breakdown of a settled order: keeps a fractional percent rate", async () => {
    stubPayment({
      getOrderSettlement: vi
        .fn()
        .mockResolvedValue({ ...resp, commissionRateBps: 250 }),
    });
    expect((await getOrderSettlement("o1"))?.commissionRatePercent).toBe(2.5);
  });

  it("Zero commission is shown, not hidden: zero commission and rate map to 0", async () => {
    stubPayment({
      getOrderSettlement: vi.fn().mockResolvedValue({
        ...resp,
        commission: 0n,
        commissionRateBps: 0,
        net: 310000n,
      }),
    });
    expect(await getOrderSettlement("o1")).toMatchObject({
      commission: 0,
      commissionRatePercent: 0,
      net: 310000,
    });
  });

  it("maps int64 settlement amounts without rounding", async () => {
    stubPayment({
      getOrderSettlement: vi.fn().mockResolvedValue({
        ...resp,
        credited: 9007199254740000n,
        commission: 450359962737000n,
        net: 9007199254740000n - 450359962737000n,
      }),
    });
    const res = await getOrderSettlement("o1");
    expect(res?.credited).toBe(9007199254740000);
    expect(res?.commission).toBe(450359962737000);
    expect(res?.net).toBe(8556839292003000);
  });

  it("Unsettled order shows the not-settled state: NOT_FOUND resolves to null", async () => {
    stubPayment({
      getOrderSettlement: vi
        .fn()
        .mockRejectedValue(new ConnectError("none", Code.NotFound)),
    });
    expect(await getOrderSettlement("o1")).toBeNull();
  });

  it("Settlement service failure degrades only the card: UNAVAILABLE rejects", async () => {
    stubPayment({
      getOrderSettlement: vi
        .fn()
        .mockRejectedValue(new ConnectError("down", Code.Unavailable)),
    });
    await expect(getOrderSettlement("o1")).rejects.toThrow();
  });

  it("Settlement service failure degrades only the card: deadline exceeded rejects", async () => {
    stubPayment({
      getOrderSettlement: vi
        .fn()
        .mockRejectedValue(new ConnectError("slow", Code.DeadlineExceeded)),
    });
    await expect(getOrderSettlement("o1")).rejects.toThrow();
  });
});
