import { revalidatePath } from "next/cache";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { ReturnStatus } from "@/generated/platform/order/v1/order_pb.js";
import { updateReturnStatus } from "@/lib/gateway/orders";

import {
  approveReturnAction,
  refundReturnAction,
  rejectReturnAction,
} from "./actions";

vi.mock("next/cache", () => ({ revalidatePath: vi.fn() }));
vi.mock("@/lib/gateway/orders", () => ({ updateReturnStatus: vi.fn() }));
// A payment refund call must never be reachable from these actions.
vi.mock("@/lib/gateway/payment", () => ({ requestWalletPayout: vi.fn() }));
vi.mock("@/lib/gateway/listings", () => ({}));
vi.mock("@/lib/gateway/promotion", () => ({}));
vi.mock("@/lib/gateway/session", () => ({ getPrincipal: vi.fn() }));

const view = {
  id: "r1",
  orderId: "o1",
  reason: "x",
  refundAmount: 1,
  status: ReturnStatus.REFUNDED,
  statusText: "Đã hoàn tiền",
};

beforeEach(() => vi.clearAllMocks());

describe("seller return actions", () => {
  it.each([
    ["approve", approveReturnAction, ReturnStatus.APPROVED],
    ["reject", rejectReturnAction, ReturnStatus.REJECTED],
    ["refund", refundReturnAction, ReturnStatus.REFUNDED],
  ] as const)(
    "%s calls updateReturnStatus only, then revalidates",
    async (_n, action, status) => {
      vi.mocked(updateReturnStatus).mockResolvedValue(view);
      const res = await action("r1", "o1");
      expect(updateReturnStatus).toHaveBeenCalledWith("r1", status);
      expect(res.ok).toBe(true);
      expect(revalidatePath).toHaveBeenCalledWith("/seller/orders/o1");
    },
  );

  it("returns the gateway error and still revalidates, so the page shows the real status", async () => {
    vi.mocked(updateReturnStatus).mockRejectedValue(
      new Error("return is not approved"),
    );
    const res = await refundReturnAction("r1", "o1");
    expect(res).toEqual({ ok: false, error: "return is not approved" });
    expect(revalidatePath).toHaveBeenCalledWith("/seller/orders/o1");
  });
});
