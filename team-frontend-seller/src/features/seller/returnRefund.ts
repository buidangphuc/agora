import { ReturnStatus } from "@/generated/platform/order/v1/order_pb.js";
import { PaymentRefundSource } from "@/generated/platform/payment/v1/payment_pb.js";
import type { ViewOrderReturn } from "@/lib/gateway/orders";
import type { ViewPaymentTransaction } from "@/lib/gateway/payment";

export const COD_RETURN_MESSAGE =
  "Đơn thanh toán khi nhận hàng (COD): việc hoàn tiền được xử lý ngoài hệ thống.";

export function formatVnd(n: number): string {
  return `${n.toLocaleString("vi-VN")}₫`;
}

export type ReturnAction = "approve" | "reject" | "refund";

/** The actions a return offers; REFUND is withheld for an order never paid online. */
export function returnActions(
  status: ReturnStatus,
  paidOnline: boolean,
): ReturnAction[] {
  switch (status) {
    case ReturnStatus.PENDING:
      return ["approve", "reject"];
    case ReturnStatus.APPROVED:
      return paidOnline ? ["refund", "reject"] : ["reject"];
    default:
      return [];
  }
}

export type RefundState =
  | { kind: "none" }
  | { kind: "processing" }
  | { kind: "refunded"; amount: number }
  | { kind: "partial"; amount: number };

/**
 * What the payment did with a REFUNDED return: it is processing until the
 * payment lists a RETURN refund with `sourceId = return.id`; then it shows the
 * applied amount, flagged partial when it is below the requested amount. An
 * unreadable payment reads as processing.
 */
export function refundState(
  ret: ViewOrderReturn,
  payment: ViewPaymentTransaction | null,
): RefundState {
  if (ret.status !== ReturnStatus.REFUNDED) return { kind: "none" };
  const refund = payment?.refunds.find(
    (r) => r.source === PaymentRefundSource.RETURN && r.sourceId === ret.id,
  );
  if (!refund) return { kind: "processing" };
  return refund.amount < refund.requestedAmount
    ? { kind: "partial", amount: refund.amount }
    : { kind: "refunded", amount: refund.amount };
}

export function refundStateText(state: RefundState): string {
  switch (state.kind) {
    case "processing":
      return "Đang xử lý hoàn tiền";
    case "refunded":
      return `Đã hoàn ${formatVnd(state.amount)}`;
    case "partial":
      return `Chỉ hoàn được ${formatVnd(state.amount)}`;
    default:
      return "";
  }
}
