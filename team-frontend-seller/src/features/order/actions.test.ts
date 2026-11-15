import { Code, ConnectError } from "@connectrpc/connect";
import { revalidatePath } from "next/cache";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { OrderStatus } from "@/generated/platform/order/v1/order_pb.js";
import { updateOrderStatus } from "@/lib/gateway/orders";
import { getPrincipal } from "@/lib/gateway/session";

import { bulkShipOrdersAction, updateOrderStatusAction } from "./actions";

vi.mock("next/cache", () => ({ revalidatePath: vi.fn() }));
vi.mock("@/lib/gateway/orders", () => ({ updateOrderStatus: vi.fn() }));
vi.mock("@/lib/gateway/session", () => ({ getPrincipal: vi.fn() }));

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

describe("bulkShipOrdersAction", () => {
  const principal = (scopes: string[]) =>
    vi.mocked(getPrincipal).mockReturnValue({
      sub: "u1",
      scopes,
    } as never);
  const items = (n: number) =>
    Array.from({ length: n }, (_, i) => ({ orderId: `o${i + 1}` }));
  const results = (res: unknown) =>
    (res as { ok: true; data: { results: Array<Record<string, unknown>> } }).data
      .results;

  beforeEach(() => principal(["listing.write"]));

  it("bulk ship sends status and tracking per order", async () => {
    vi.mocked(updateOrderStatus).mockResolvedValue({} as never);
    const res = await bulkShipOrdersAction([
      { orderId: "o1", trackingNumber: " T1 " },
      { orderId: "o2", trackingNumber: "T2" },
      { orderId: "o3" },
    ]);
    expect(vi.mocked(updateOrderStatus).mock.calls).toEqual([
      ["o1", OrderStatus.SHIPPED, "T1"],
      ["o2", OrderStatus.SHIPPED, "T2"],
      ["o3", OrderStatus.SHIPPED, ""],
    ]);
    expect(res.ok).toBe(true);
    expect(results(res).map((r) => [r.orderId, r.ok])).toEqual([
      ["o1", true],
      ["o2", true],
      ["o3", true],
    ]);
    const paths = vi.mocked(revalidatePath).mock.calls.map((c) => c[0]);
    expect(paths.sort()).toEqual(["/seller", "/seller/orders"]);
  });

  it("one failure does not stop the others", async () => {
    vi.mocked(updateOrderStatus).mockImplementation(async (id) => {
      if (id === "o2") throw new ConnectError("cancelled", Code.FailedPrecondition);
      return {} as never;
    });
    const res = await bulkShipOrdersAction(items(3));
    expect(updateOrderStatus).toHaveBeenCalledTimes(3);
    const r = results(res);
    expect(r[0]).toMatchObject({ orderId: "o1", ok: true });
    expect(r[1]).toMatchObject({
      orderId: "o2",
      ok: false,
      error: "Trạng thái đơn không cho phép gửi (có thể đã huỷ hoặc đã gửi)",
    });
    expect(r[2]).toMatchObject({ orderId: "o3", ok: true });
  });

  it("foreign order is denied per row", async () => {
    vi.mocked(updateOrderStatus).mockImplementation(async (id) => {
      if (id === "o2") throw new ConnectError("seller x owns it", Code.PermissionDenied);
      return {} as never;
    });
    const res = await bulkShipOrdersAction(items(3));
    const r = results(res);
    expect(r[1]).toMatchObject({ ok: false, error: "Không có quyền với đơn này" });
    expect(JSON.stringify(res)).not.toContain("seller x owns it");
    expect(r[0]).toMatchObject({ ok: true });
    expect(r[2]).toMatchObject({ ok: true });
  });

  it("unavailable maps to a busy reason", async () => {
    vi.mocked(updateOrderStatus).mockRejectedValue(
      new ConnectError("dial tcp", Code.Unavailable),
    );
    const res = await bulkShipOrdersAction(items(2));
    expect(results(res).map((r) => r.error)).toEqual([
      "Dịch vụ đơn hàng đang bận",
      "Dịch vụ đơn hàng đang bận",
    ]);
    expect(JSON.stringify(res)).not.toContain("dial tcp");
  });

  it("51 ids rejected without calls", async () => {
    const res = await bulkShipOrdersAction(items(51));
    expect(res).toEqual({ ok: false, error: "Chọn tối đa 50 đơn" });
    expect(updateOrderStatus).not.toHaveBeenCalled();
    expect(revalidatePath).not.toHaveBeenCalled();
  });

  it("empty batch rejected", async () => {
    const res = await bulkShipOrdersAction([]);
    expect(res.ok).toBe(false);
    expect(updateOrderStatus).not.toHaveBeenCalled();
  });

  it("empty order id rejected", async () => {
    const res = await bulkShipOrdersAction([{ orderId: "  " }]);
    expect(res.ok).toBe(false);
    expect(updateOrderStatus).not.toHaveBeenCalled();
  });

  it("duplicate ids collapse", async () => {
    vi.mocked(updateOrderStatus).mockResolvedValue({} as never);
    const res = await bulkShipOrdersAction([{ orderId: "o1" }, { orderId: "o1" }]);
    expect(updateOrderStatus).toHaveBeenCalledTimes(1);
    expect(results(res)).toHaveLength(1);
  });

  it("scope required before any call", async () => {
    vi.mocked(getPrincipal).mockReturnValue(null);
    const anon = await bulkShipOrdersAction(items(2));
    principal(["order.read"]);
    const noScope = await bulkShipOrdersAction(items(2));
    expect(anon.ok).toBe(false);
    expect(noScope.ok).toBe(false);
    expect(updateOrderStatus).not.toHaveBeenCalled();
    expect(revalidatePath).not.toHaveBeenCalled();
  });
});
