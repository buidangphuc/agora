import { beforeEach, describe, expect, it, vi } from "vitest";

import {
  PaymentMethod,
  PaymentStatus,
} from "@/generated/platform/payment/v1/payment_pb.js";

import { makeClients } from "./client.js";
import {
  createPayment,
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
};

beforeEach(() => vi.clearAllMocks());

describe("payment label helpers", () => {
  it("maps method + status enums to Vietnamese labels", () => {
    expect(getPaymentMethodText(PaymentMethod.COD)).toContain("COD");
    expect(getPaymentStatusText(PaymentStatus.PAID)).toBe("Đã thanh toán");
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
