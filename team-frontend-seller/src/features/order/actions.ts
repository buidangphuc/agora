"use server";

import { revalidatePath } from "next/cache";

import type { OrderStatus } from "@/generated/platform/order/v1/order_pb.js";
import { type ActionResult, fail, ok } from "@/lib/action-result";
import { updateOrderStatus } from "@/lib/gateway/orders";

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
