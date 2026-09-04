import { beforeEach, describe, expect, it, vi } from "vitest";

import { ReturnStatus } from "@/generated/platform/order/v1/order_pb.js";
import { createReturnRequest, updateReturnStatus } from "@/lib/gateway/orders";
import { refundPayment } from "@/lib/gateway/payment";

import { createReturnRequestAction, mockRefundAction } from "./actions";

vi.mock("@/lib/gateway/orders", () => ({
  createReturnRequest: vi.fn(),
  updateReturnStatus: vi.fn(),
}));
vi.mock("@/lib/gateway/payment", () => ({
  refundPayment: vi.fn(),
}));

beforeEach(() => vi.clearAllMocks());

describe("createReturnRequestAction", () => {
  it("rejects an empty reason without calling the gateway", async () => {
    const res = await createReturnRequestAction("o1", "  ", 1000);
    expect(res.ok).toBe(false);
    expect(createReturnRequest).not.toHaveBeenCalled();
  });

  it("rejects a non-positive amount", async () => {
    const res = await createReturnRequestAction("o1", "hỏng", 0);
    expect(res.ok).toBe(false);
    expect(createReturnRequest).not.toHaveBeenCalled();
  });

  it("creates the return request and returns its view", async () => {
    vi.mocked(createReturnRequest).mockResolvedValue({
      id: "r1",
      orderId: "o1",
      reason: "hỏng",
      refundAmount: 1000,
      status: ReturnStatus.PENDING,
      statusText: "Chờ duyệt",
    });
    const res = await createReturnRequestAction("o1", " hỏng ", 1000);
    expect(createReturnRequest).toHaveBeenCalledWith("o1", "hỏng", 1000);
    expect(res.ok).toBe(true);
    expect(res.returnRequest?.id).toBe("r1");
  });

  it("returns an error shape when the gateway throws", async () => {
    vi.mocked(createReturnRequest).mockRejectedValue(new Error("boom"));
    const res = await createReturnRequestAction("o1", "hỏng", 1000);
    expect(res).toEqual({ ok: false, message: "boom" });
  });
});

describe("mockRefundAction", () => {
  it("marks the return REFUNDED and runs the mock refund leg", async () => {
    vi.mocked(updateReturnStatus).mockResolvedValue({
      id: "r1",
      orderId: "o1",
      reason: "hỏng",
      refundAmount: 1000,
      status: ReturnStatus.REFUNDED,
      statusText: "Đã hoàn tiền",
    });
    vi.mocked(refundPayment).mockResolvedValue({
      ok: true,
      orderId: "o1",
      amount: 1000,
      message: "ok",
    });
    const res = await mockRefundAction("r1", "o1", 1000);
    expect(updateReturnStatus).toHaveBeenCalledWith(
      "r1",
      ReturnStatus.REFUNDED,
    );
    expect(refundPayment).toHaveBeenCalledWith("o1", 1000);
    expect(res.ok).toBe(true);
    expect(res.returnRequest?.status).toBe(ReturnStatus.REFUNDED);
  });

  it("returns an error shape when the status update fails", async () => {
    vi.mocked(updateReturnStatus).mockRejectedValue(new Error("nope"));
    const res = await mockRefundAction("r1", "o1", 1000);
    expect(res).toEqual({ ok: false, message: "nope" });
    expect(refundPayment).not.toHaveBeenCalled();
  });
});
