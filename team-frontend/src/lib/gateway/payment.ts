import "server-only";

import {
  PaymentMethod,
  PaymentStatus,
  type PaymentTransaction,
} from "@/generated/platform/payment/v1/payment_pb.js";
import { makeClients } from "./client.js";
import { getToken } from "./session.js";

function gateway() {
  return makeClients(getToken());
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
    case PaymentStatus.REFUNDED:
      return "Đã hoàn tiền";
    default:
      return "Chưa xác định";
  }
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

export interface ViewRefundResult {
  ok: boolean;
  orderId: string;
  amount: number;
  message: string;
}

/**
 * MOCK refund (AGENTS.md §7 — no real money movement). The payment service does
 * not expose a RefundPayment RPC; the authoritative refund state lives on the
 * OrderReturn (ReturnStatus.REFUNDED via order.updateReturnStatus). This helper
 * simulates the money-back leg for the UI so the return flow reads end-to-end
 * without wiring a real payment gateway.
 */
export async function refundPayment(
  orderId: string,
  amount: number,
): Promise<ViewRefundResult> {
  return {
    ok: true,
    orderId,
    amount,
    message: "Hoàn tiền (mô phỏng) thành công.",
  };
}

// ── Seller wallet (balance + ledger + payout) ───────────────────────────────

export interface ViewWalletEntry {
  id: string;
  sellerId: string;
  type: string;
  amount: number;
  status: string;
  createdAt: string;
}

function mapWalletEntry(e: {
  id: string;
  sellerId: string;
  type: string;
  amount: bigint;
  status: string;
  createdAt?: { seconds: bigint };
}): ViewWalletEntry {
  let createdAt = "";
  if (e.createdAt) {
    createdAt = new Date(
      Number(e.createdAt.seconds) * 1000,
    ).toLocaleDateString("vi-VN");
  }
  return {
    id: e.id,
    sellerId: e.sellerId,
    type: e.type,
    amount: Number(e.amount),
    status: e.status,
    createdAt,
  };
}

export async function getWalletBalance(sellerId: string): Promise<number> {
  try {
    const res = await gateway().payment.getWalletBalance({ sellerId });
    return Number(res.balance);
  } catch {
    return 0;
  }
}

export async function listLedgerEntries(
  sellerId: string,
): Promise<ViewWalletEntry[]> {
  try {
    const res = await gateway().payment.listLedgerEntries({
      sellerId,
      page: { cursor: "", pageSize: 30 },
    });
    return res.entries.map(mapWalletEntry);
  } catch {
    return [];
  }
}

export async function requestWalletPayout(
  sellerId: string,
  amount: number,
): Promise<ViewWalletEntry | null> {
  const res = await gateway().payment.requestWalletPayout({
    sellerId,
    amount: BigInt(Math.max(0, Math.round(amount))),
  });
  return res.entry ? mapWalletEntry(res.entry) : null;
}
