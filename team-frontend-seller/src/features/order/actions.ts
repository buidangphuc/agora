"use server";

import { revalidatePath } from "next/cache";

import { OrderStatus } from "@/generated/platform/order/v1/order_pb.js";
import { type ActionResult, fail, ok } from "@/lib/action-result";
import { updateOrderStatus } from "@/lib/gateway/orders";
import { getPrincipal } from "@/lib/gateway/session";

import {
  BULK_SHIP_MAX,
  NOT_ATTEMPTED_REASON,
  reasonForError,
  runPool,
} from "../seller/bulkShip";

/**
 * Seller moves an order along (ship, ...). Shared mutation contract: returns
 * `{ ok, error?, data? }` and revalidates the seller list and the detail page.
 */
export async function updateOrderStatusAction(
  orderId: string,
  status: OrderStatus,
  trackingNumber?: string,
): Promise<ActionResult<{ status: OrderStatus; trackingNumber: string }>> {
  try {
    const order = await updateOrderStatus(orderId, status, trackingNumber);
    revalidatePath("/seller/orders");
    revalidatePath(`/seller/orders/${orderId}`);
    return ok({
      status: order?.status ?? status,
      trackingNumber: order?.trackingNumber ?? trackingNumber ?? "",
    });
  } catch (err: unknown) {
    return fail(err instanceof Error ? err.message : "Cập nhật thất bại.");
  }
}

export interface BulkShipRow {
  orderId: string;
  ok: boolean;
  error?: string;
}

export async function bulkShipOrdersAction(
  orders: Array<{ orderId: string; trackingNumber?: string }>,
): Promise<ActionResult<{ results: BulkShipRow[] }>> {
  const me = getPrincipal();
  if (!me) return fail("Bạn cần đăng nhập để gửi hàng.");
  if (!me.scopes.includes("listing.write")) {
    return fail("Bạn không có quyền gửi hàng.");
  }
  const distinct = new Map<string, string>();
  for (const o of orders) {
    const id = (o.orderId ?? "").trim();
    if (!id) return fail("Mã đơn không hợp lệ");
    if (!distinct.has(id)) distinct.set(id, (o.trackingNumber ?? "").trim());
  }
  if (distinct.size === 0) return fail("Chưa chọn đơn nào");
  if (distinct.size > BULK_SHIP_MAX) return fail("Chọn tối đa 50 đơn");

  const entries = [...distinct.entries()];
  const pooled = await runPool(entries, async ([id, tracking]) => {
    try {
      await updateOrderStatus(id, OrderStatus.SHIPPED, tracking);
      return { orderId: id, ok: true } as BulkShipRow;
    } catch (err: unknown) {
      return {
        orderId: id,
        ok: false,
        error: reasonForError(err),
      } as BulkShipRow;
    }
  });
  const results = pooled.map((r, i) =>
    r.done
      ? r.value
      : { orderId: entries[i][0], ok: false, error: NOT_ATTEMPTED_REASON },
  );
  revalidatePath("/seller/orders");
  revalidatePath("/seller");
  return ok({ results });
}
