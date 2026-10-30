import "server-only";

import {
  PaymentMethod,
  type PaymentRefund,
  PaymentRefundSource,
  PaymentStatus,
  type PaymentTransaction,
} from "@/generated/platform/payment/v1/payment_pb.js";
import { makeClients } from "./client.js";
import { getToken } from "./session.js";

function gateway() {
  return makeClients(getToken());
}

export interface ViewPaymentRefund {
  id: string;
  source: PaymentRefundSource;
  sourceId: string;
  /** What was asked for; `amount` is what the payment applied (may be lower, or 0). */
  requestedAmount: number;
  amount: number;
  reason: string;
}

export interface ViewPaymentTransaction {
  id: string;
  orderId: string;
  buyerId: string;
  amount: number;
  currency: string;
  method: PaymentMethod;
  methodText: string;
  status: PaymentStatus;
  statusText: string;
  providerReference: string;
  createdAt: string;
  refundedAmount: number;
  refunds: ViewPaymentRefund[];
}

export function getPaymentMethodText(method: PaymentMethod): string {
  switch (method) {
    case PaymentMethod.COD:
      return "Thanh toán khi nhận hàng (COD)";
    case PaymentMethod.MOCK_MOMO:
      return "Ví điện tử MoMo (Demo)";
    case PaymentMethod.MOCK_BANK:
      return "Chuyển khoản Ngân hàng (Demo)";
    case PaymentMethod.MOCK_CARD:
      return "Thẻ Tín dụng / Ghi nợ (Demo)";
    default:
      return "Chưa xác định";
  }
}

export function getPaymentStatusText(status: PaymentStatus): string {
  switch (status) {
    case PaymentStatus.PENDING:
      return "Chờ thanh toán";
    case PaymentStatus.PAID:
      return "Đã thanh toán";
    case PaymentStatus.FAILED:
      return "Thanh toán thất bại";
    case PaymentStatus.PARTIALLY_REFUNDED:
      return "Đã hoàn một phần";
    case PaymentStatus.REFUNDED:
      return "Đã hoàn tiền";
    case PaymentStatus.CANCELLED:
      return "Đã hủy";
    default:
      return "Chưa xác định";
  }
}

function mapRefund(r: PaymentRefund): ViewPaymentRefund {
  return {
    id: r.id,
    source: r.source,
    sourceId: r.sourceId,
    requestedAmount: Number(r.requestedAmount),
    amount: Number(r.amount),
    reason: r.reason,
  };
}

function mapTransaction(t: PaymentTransaction): ViewPaymentTransaction {
  let createdAt = "";
  if (t.createdAt) {
    createdAt = new Date(Number(t.createdAt.seconds) * 1000).toLocaleString(
      "vi-VN",
    );
  }
  return {
    id: t.id,
    orderId: t.orderId,
    buyerId: t.buyerId,
    amount: Number(t.amount),
    currency: t.currency || "VND",
    method: t.method,
    methodText: getPaymentMethodText(t.method),
    status: t.status,
    statusText: getPaymentStatusText(t.status),
    providerReference: t.providerReference,
    createdAt,
    refundedAmount: Number(t.refundedAmount),
    refunds: t.refunds.map(mapRefund),
  };
}

export async function createPayment(
  orderId: string,
  method: PaymentMethod,
): Promise<{ transaction: ViewPaymentTransaction; paymentUrl: string }> {
  const res = await gateway().payment.createPayment({
    orderId,
    method,
  });
  if (!res.transaction) throw new Error("create payment failed");
  return {
    transaction: mapTransaction(res.transaction),
    paymentUrl: res.paymentUrl,
  };
}

export async function getPayment(
  id?: string,
  orderId?: string,
): Promise<ViewPaymentTransaction | null> {
  try {
    const res = await gateway().payment.getPayment({
      id: id ?? "",
      orderId: orderId ?? "",
    });
    return res.transaction ? mapTransaction(res.transaction) : null;
  } catch {
    return null;
  }
}

export async function processMockPayment(
  transactionId: string,
  simulateSuccess: boolean,
): Promise<{
  transaction: ViewPaymentTransaction;
  success: boolean;
  message: string;
}> {
  const res = await gateway().payment.processMockPayment({
    transactionId,
    simulateSuccess,
  });
  if (!res.transaction) throw new Error("process mock payment failed");
  return {
    transaction: mapTransaction(res.transaction),
    success: res.success,
    message: res.message,
  };
}
