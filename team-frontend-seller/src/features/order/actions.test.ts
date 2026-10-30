import { revalidatePath } from "next/cache";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { OrderStatus } from "@/generated/platform/order/v1/order_pb.js";
import { updateOrderStatus } from "@/lib/gateway/orders";

import { updateOrderStatusAction } from "./actions";

vi.mock("next/cache", () => ({ revalidatePath: vi.fn() }));
vi.mock("@/lib/gateway/orders", () => ({ updateOrderStatus: vi.fn() }));

beforeEach(() => {
  vi.clearAllMocks();
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
