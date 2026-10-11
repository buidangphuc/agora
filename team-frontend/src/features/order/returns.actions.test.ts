import { revalidatePath } from "next/cache";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { ReturnStatus } from "@/generated/platform/order/v1/order_pb.js";
import { createReturnRequest } from "@/lib/gateway/orders";

import { createReturnRequestAction } from "./actions";

vi.mock("next/cache", () => ({ revalidatePath: vi.fn() }));
vi.mock("@/lib/gateway/orders", () => ({
  createReturnRequest: vi.fn(),
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
    expect(res.ok && res.data?.id).toBe("r1");
    expect(revalidatePath).toHaveBeenCalledWith("/account/orders");
    expect(revalidatePath).toHaveBeenCalledWith("/account/orders/o1");
  });

  it("returns an error shape when the gateway throws", async () => {
    vi.mocked(createReturnRequest).mockRejectedValue(new Error("boom"));
    const res = await createReturnRequestAction("o1", "hỏng", 1000);
    expect(res).toEqual({ ok: false, error: "boom" });
  });
});
