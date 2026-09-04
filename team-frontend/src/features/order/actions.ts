"use server";

import { revalidatePath } from "next/cache";

import type { OrderStatus } from "@/generated/platform/order/v1/order_pb.js";
import { ReturnStatus } from "@/generated/platform/order/v1/order_pb.js";
import type { PaymentMethod } from "@/generated/platform/payment/v1/payment_pb.js";
import { reorder } from "@/lib/gateway/cart";
import {
  type ViewOrderReturn,
  cancelOrder,
  createOrder,
  createReturnRequest,
  updateOrderStatus,
  updateReturnStatus,
} from "@/lib/gateway/orders";
import {
  createPayment,
  processMockPayment,
  refundPayment,
} from "@/lib/gateway/payment";

export interface OrderActionResult {
  ok: boolean;
  message?: string;
  orderIds?: string[];
  paymentUrl?: string;
}

export async function checkoutAction(
  addressId?: string,
  itemIds?: string[],
  paymentMethod?: PaymentMethod,
  voucherCode?: string,
): Promise<OrderActionResult> {
  try {
    const orders = await createOrder(
      addressId,
      itemIds,
      paymentMethod,
      voucherCode,
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

    return {
      ok: true,
      orderIds,
      paymentUrl,
      message: "Đặt hàng thành công!",
    };
  } catch (err: unknown) {
    return {
      ok: false,
      message: err instanceof Error ? err.message : "Đặt hàng thất bại.",
    };
  }
}

export async function updateOrderStatusAction(
  orderId: string,
  status: OrderStatus,
  trackingNumber?: string,
): Promise<OrderActionResult> {
  try {
    await updateOrderStatus(orderId, status, trackingNumber);
    revalidatePath("/account/orders");
    revalidatePath("/seller/orders");
    return { ok: true, message: "Cập nhật trạng thái đơn hàng thành công!" };
  } catch (err: unknown) {
    return {
      ok: false,
      message: err instanceof Error ? err.message : "Cập nhật thất bại.",
    };
  }
}

export async function cancelOrderAction(
  orderId: string,
  reason?: string,
): Promise<OrderActionResult> {
  try {
    await cancelOrder(orderId, reason);
    revalidatePath("/account/orders");
    revalidatePath("/seller/orders");
    return { ok: true, message: "Đã hủy đơn hàng thành công." };
  } catch (err: unknown) {
    return {
      ok: false,
      message: err instanceof Error ? err.message : "Hủy đơn hàng thất bại.",
    };
  }
}

/** Re-add all items of a past order back into the cart ("Mua lại"). */
export async function reorderAction(
  orderId: string,
): Promise<{ ok: boolean; totalItems?: number; message?: string }> {
  try {
    const cart = await reorder(orderId);
    revalidatePath("/cart");
    return { ok: true, totalItems: cart.totalItems };
  } catch (err: unknown) {
    return {
      ok: false,
      message: err instanceof Error ? err.message : "Mua lại thất bại.",
    };
  }
}

export async function createPaymentAction(
  orderId: string,
  method: PaymentMethod,
): Promise<{ ok: boolean; paymentUrl?: string; message?: string }> {
  try {
    const res = await createPayment(orderId, method);
    return { ok: true, paymentUrl: res.paymentUrl };
  } catch (err: unknown) {
    return {
      ok: false,
      message:
        err instanceof Error ? err.message : "Khởi tạo thanh toán thất bại.",
    };
  }
}

export async function processMockPaymentAction(
  transactionId: string,
  simulateSuccess: boolean,
): Promise<{ ok: boolean; message?: string }> {
  try {
    const res = await processMockPayment(transactionId, simulateSuccess);
    revalidatePath("/account/orders");
    revalidatePath("/seller/orders");
    return { ok: res.success, message: res.message };
  } catch (err: unknown) {
    return {
      ok: false,
      message:
        err instanceof Error ? err.message : "Xử lý thanh toán thất bại.",
    };
  }
}

export interface ReturnActionResult {
  ok: boolean;
  message?: string;
  returnRequest?: ViewOrderReturn;
}

export async function createReturnRequestAction(
  orderId: string,
  reason: string,
  refundAmount: number,
): Promise<ReturnActionResult> {
  if (!reason.trim()) {
    return { ok: false, message: "Vui lòng nhập lý do trả hàng." };
  }
  if (!(refundAmount > 0)) {
    return { ok: false, message: "Số tiền hoàn phải lớn hơn 0." };
  }
  try {
    const returnRequest = await createReturnRequest(
      orderId,
      reason.trim(),
      refundAmount,
    );
    revalidatePath(`/account/orders/${orderId}`);
    return {
      ok: true,
      returnRequest,
      message: "Đã gửi yêu cầu trả hàng / hoàn tiền.",
    };
  } catch (err: unknown) {
    return {
      ok: false,
      message:
        err instanceof Error ? err.message : "Gửi yêu cầu trả hàng thất bại.",
    };
  }
}

/**
 * MOCK refund: mark the return REFUNDED (authoritative state on the order) and
 * simulate the money-back leg via the mock payment helper. No real money moves
 * (AGENTS.md §7).
 */
export async function mockRefundAction(
  returnId: string,
  orderId: string,
  amount: number,
): Promise<ReturnActionResult> {
  try {
    const returnRequest = await updateReturnStatus(
      returnId,
      ReturnStatus.REFUNDED,
    );
    await refundPayment(orderId, amount);
    revalidatePath(`/account/orders/${orderId}`);
    return {
      ok: true,
      returnRequest,
      message: "Hoàn tiền (mô phỏng) thành công.",
    };
  } catch (err: unknown) {
    return {
      ok: false,
      message: err instanceof Error ? err.message : "Hoàn tiền thất bại.",
    };
  }
}
