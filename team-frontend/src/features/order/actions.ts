"use server";

import { Code, ConnectError } from "@connectrpc/connect";
import { revalidatePath } from "next/cache";

import type { OrderStatus } from "@/generated/platform/order/v1/order_pb.js";
import type { PaymentMethod } from "@/generated/platform/payment/v1/payment_pb.js";
import { type ActionResult, fail, ok } from "@/lib/action-result";
import { reorder } from "@/lib/gateway/cart";
import {
  type ViewOrderReturn,
  cancelOrder,
  createOrder,
  createReturnRequest,
  updateOrderStatus,
} from "@/lib/gateway/orders";
import { createPayment, processMockPayment } from "@/lib/gateway/payment";

export interface OrderActionResult {
  ok: boolean;
  message?: string;
  orderIds?: string[];
  paymentUrl?: string;
}

export interface CheckoutData {
  orderIds: string[];
  paymentUrl?: string;
}

const CHECKOUT_IN_PROGRESS =
  "Đơn hàng đang được xử lý, vui lòng thử lại sau giây lát.";

export async function checkoutAction(
  addressId?: string,
  itemIds?: string[],
  paymentMethod?: PaymentMethod,
  voucherCode?: string,
  idempotencyKey?: string,
): Promise<ActionResult<CheckoutData>> {
  try {
    const orders = await createOrder(
      addressId,
      itemIds,
      paymentMethod,
      voucherCode,
      idempotencyKey,
    );
    revalidatePath("/cart");
    revalidatePath("/checkout");
    revalidatePath("/account/orders");
    revalidatePath("/seller/orders");

    const orderIds = orders.map((o) => o.id);
    let paymentUrl: string | undefined;

    // If online payment method selected and we have orders, initiate payment transaction for the first order
    if (orders.length > 0 && paymentMethod && paymentMethod !== 1) {
      // 1 = COD
      try {
        const paymentRes = await createPayment(orders[0].id, paymentMethod);
        paymentUrl = paymentRes.paymentUrl;
      } catch {
        // Fall back to direct order view
      }
    }

    return ok({ orderIds, paymentUrl });
  } catch (err: unknown) {
    // `aborted`: the same idempotency key is still being processed by a
    // concurrent request. Nothing failed; the buyer can safely retry.
    if (err instanceof ConnectError && err.code === Code.Aborted) {
      return fail(CHECKOUT_IN_PROGRESS);
    }
    return fail(err instanceof Error ? err.message : "Đặt hàng thất bại.");
  }
}

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
    revalidatePath("/account/orders");
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

/** Revalidate the buyer list and the detail page of one order. */
function revalidateBuyerOrder(orderId: string) {
  revalidatePath("/account/orders");
  revalidatePath(`/account/orders/${orderId}`);
}

export async function cancelOrderAction(
  orderId: string,
  reason?: string,
): Promise<ActionResult> {
  try {
    await cancelOrder(orderId, reason);
    revalidateBuyerOrder(orderId);
    revalidatePath("/seller/orders");
    return ok();
  } catch (err: unknown) {
    return fail(err instanceof Error ? err.message : "Hủy đơn hàng thất bại.");
  }
}

/** Re-add all items of a past order back into the cart ("Mua lại"). */
export async function reorderAction(
  orderId: string,
): Promise<ActionResult<{ totalItems: number }>> {
  try {
    const cart = await reorder(orderId);
    revalidatePath("/cart");
    revalidateBuyerOrder(orderId);
    return ok({ totalItems: cart.totalItems });
  } catch (err: unknown) {
    return fail(err instanceof Error ? err.message : "Mua lại thất bại.");
  }
}

export async function createPaymentAction(
  orderId: string,
  method: PaymentMethod,
): Promise<ActionResult<{ paymentUrl: string }>> {
  try {
    const res = await createPayment(orderId, method);
    return ok({ paymentUrl: res.paymentUrl });
  } catch (err: unknown) {
    return fail(
      err instanceof Error ? err.message : "Khởi tạo thanh toán thất bại.",
    );
  }
}

export async function processMockPaymentAction(
  transactionId: string,
  simulateSuccess: boolean,
): Promise<ActionResult<{ message: string }>> {
  try {
    const res = await processMockPayment(transactionId, simulateSuccess);
    revalidatePath("/account/orders");
    revalidatePath("/seller/orders");
    return res.success
      ? ok({ message: res.message })
      : fail(res.message || "Thanh toán thất bại.");
  } catch (err: unknown) {
    return fail(
      err instanceof Error ? err.message : "Xử lý thanh toán thất bại.",
    );
  }
}

export async function createReturnRequestAction(
  orderId: string,
  reason: string,
  refundAmount: number,
): Promise<ActionResult<ViewOrderReturn>> {
  if (!reason.trim()) {
    return fail("Vui lòng nhập lý do trả hàng.");
  }
  if (!(refundAmount > 0)) {
    return fail("Số tiền hoàn phải lớn hơn 0.");
  }
  try {
    const returnRequest = await createReturnRequest(
      orderId,
      reason.trim(),
      refundAmount,
    );
    revalidateBuyerOrder(orderId);
    return ok(returnRequest);
  } catch (err: unknown) {
    return fail(
      err instanceof Error ? err.message : "Gửi yêu cầu trả hàng thất bại.",
    );
  }
}
