"use server";

import { Code, ConnectError } from "@connectrpc/connect";
import { revalidatePath } from "next/cache";

import type { OrderStatus } from "@/generated/platform/order/v1/order_pb.js";
import type { PaymentMethod } from "@/generated/platform/payment/v1/payment_pb.js";
import { type ActionResult, fail, ok } from "@/lib/action-result";
import { reorder } from "@/lib/gateway/cart";
import {
  type ConfirmedAddressInput,
  type ShopVoucherInput,
  type ViewOrderReturn,
  cancelOrder,
  confirmReceipt,
  createOrder,
  createReturnRequest,
  updateOrderStatus,
} from "@/lib/gateway/orders";
import { createPayment, processMockPayment } from "@/lib/gateway/payment";
import { findCancelReason } from "./cancelReasons";
import {
  ADDRESS_CHANGED_MESSAGE,
  ADDRESS_NOT_FOUND_MESSAGE,
  PRICE_CHANGED_MESSAGE,
  UNAVAILABLE_RETRY_MESSAGE,
} from "./shippingCopy";

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

export interface CheckoutInput {
  addressId?: string;
  itemIds?: string[];
  paymentMethod?: PaymentMethod;
  shopVouchers?: ShopVoucherInput[];
  platformVoucherCode?: string;
  confirmedAddress?: ConfirmedAddressInput;
  idempotencyKey?: string;
}

export type CheckoutResult =
  | { ok: true; data?: CheckoutData }
  | {
      ok: false;
      error: string;
      priceChanged?: boolean;
      addressChanged?: boolean;
      addressNotFound?: boolean;
    };

export async function checkoutAction(
  input: CheckoutInput,
): Promise<CheckoutResult> {
  try {
    const orders = await createOrder(
      input.addressId,
      input.itemIds,
      input.paymentMethod,
      input.idempotencyKey,
      {
        shopVouchers: input.shopVouchers,
        platformVoucherCode: input.platformVoucherCode,
        confirmedAddress: input.confirmedAddress,
      },
    );
    revalidatePath("/cart");
    revalidatePath("/checkout");
    revalidatePath("/account/orders");
    revalidatePath("/seller/orders");

    const orderIds = orders.map((o) => o.id);
    const online =
      orders.length > 0 && input.paymentMethod && input.paymentMethod !== 1;
    const paymentUrl = online ? `/checkout/pay/${orders[0].id}` : undefined;

    return ok({ orderIds, paymentUrl });
  } catch (err: unknown) {
    if (err instanceof ConnectError && err.code === Code.Aborted) {
      return fail(CHECKOUT_IN_PROGRESS);
    }
    if (err instanceof ConnectError && err.code === Code.Unavailable) {
      return fail(UNAVAILABLE_RETRY_MESSAGE);
    }
    if (err instanceof ConnectError && err.code === Code.FailedPrecondition) {
      if (err.rawMessage.includes("price changed")) {
        revalidatePath("/cart");
        revalidatePath("/checkout");
        return { ok: false, error: PRICE_CHANGED_MESSAGE, priceChanged: true };
      }
      if (err.rawMessage.includes("address changed")) {
        revalidatePath("/checkout");
        return {
          ok: false,
          error: ADDRESS_CHANGED_MESSAGE,
          addressChanged: true,
        };
      }
      if (err.rawMessage.includes("address not found")) {
        return {
          ok: false,
          error: ADDRESS_NOT_FOUND_MESSAGE,
          addressNotFound: true,
        };
      }
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
  const choice = findCancelReason(reason);
  if (!choice) return fail("Lý do hủy không hợp lệ.");
  try {
    await cancelOrder(orderId, choice.label, choice.code);
    revalidateBuyerOrder(orderId);
    revalidatePath("/seller/orders");
    return ok();
  } catch (err: unknown) {
    return fail(err instanceof Error ? err.message : "Hủy đơn hàng thất bại.");
  }
}

const ORDER_ALREADY_COMPLETED = "Đơn hàng đã được hoàn thành.";

/** Buyer confirms a shipped order arrived; the order completes. */
export async function confirmReceiptAction(
  orderId: string,
): Promise<ActionResult> {
  try {
    await confirmReceipt(orderId);
    revalidateBuyerOrder(orderId);
    return ok();
  } catch (err: unknown) {
    // Auto-complete won the race: the order is no longer shipped.
    if (err instanceof ConnectError && err.code === Code.FailedPrecondition) {
      revalidateBuyerOrder(orderId);
      return fail(ORDER_ALREADY_COMPLETED);
    }
    return fail(err instanceof Error ? err.message : "Xác nhận thất bại.");
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

const RETURN_WINDOW_CLOSED =
  "Đã hết thời hạn trả hàng (15 ngày kể từ khi đơn hoàn thành).";

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
    if (
      err instanceof ConnectError &&
      err.code === Code.FailedPrecondition &&
      err.rawMessage.includes("return window closed")
    ) {
      return fail(RETURN_WINDOW_CLOSED);
    }
    return fail(
      err instanceof Error ? err.message : "Gửi yêu cầu trả hàng thất bại.",
    );
  }
}
