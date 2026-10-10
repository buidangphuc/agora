"use client";

import React, { useState } from "react";

import { Alert } from "@/components/ui/Alert";
import { Button } from "@/components/ui/Button";
import { Descriptions } from "@/components/ui/Descriptions";
import { Empty } from "@/components/ui/Empty";
import { Modal } from "@/components/ui/Modal";
import { useToast } from "@/components/ui/ToastProvider";
import { OrderStatusBadge } from "@/features/order/OrderStatusBadge";
import { ReturnStatus } from "@/generated/platform/order/v1/order_pb.js";
import type { ActionResult } from "@/lib/action-result";
import type { ViewOrderReturn } from "@/lib/gateway/orders";
import type { ViewPaymentTransaction } from "@/lib/gateway/payment";
import {
  approveReturnAction,
  refundReturnAction,
  rejectReturnAction,
} from "./actions";
import {
  COD_RETURN_MESSAGE,
  type ReturnAction,
  formatVnd,
  refundState,
  refundStateText,
  returnActions,
} from "./returnRefund";
import { usePending } from "./usePending";

/**
 * The seller's returns tab: each return with its status, the actions its status
 * allows, and what the payment actually applied. Every action goes through
 * `UpdateReturnStatus` only; the page is re-rendered from server data (the
 * actions revalidate it), so there is no local copy of a return to go stale.
 */
export function SellerReturns({
  orderId,
  paidOnline,
  returns,
  payment,
}: {
  orderId: string;
  /** False when the order has no `paid_at` (e.g. cash on delivery). */
  paidOnline: boolean;
  returns: ViewOrderReturn[];
  /** Null when the payment could not be read. */
  payment: ViewPaymentTransaction | null;
}) {
  const toast = useToast();
  const { pending, run } = usePending();
  const [busyId, setBusyId] = useState("");
  const [confirmId, setConfirmId] = useState("");

  async function act(action: ReturnAction, ret: ViewOrderReturn) {
    if (pending) return;
    setBusyId(ret.id);
    const call: Record<
      ReturnAction,
      (id: string, order: string) => Promise<ActionResult<ViewOrderReturn>>
    > = {
      approve: approveReturnAction,
      reject: rejectReturnAction,
      refund: refundReturnAction,
    };
    const res = await run(() => call[action](ret.id, orderId));
    setBusyId("");
    setConfirmId("");
    if (res.ok) {
      toast.success(
        action === "refund"
          ? "Đã gửi yêu cầu hoàn tiền."
          : action === "approve"
            ? "Đã duyệt yêu cầu trả hàng."
            : "Đã từ chối yêu cầu trả hàng.",
      );
    } else {
      toast.error(res.error);
    }
  }

  const confirming = returns.find((r) => r.id === confirmId);

  return (
    <div data-testid="seller-returns" className="space-y-4">
      {payment ? (
        <div data-testid="payment-summary">
          <Descriptions
            title="Thanh toán"
            column={2}
            items={[
              {
                key: "status",
                label: "Trạng thái",
                children: payment.statusText,
              },
              {
                key: "refunded",
                label: "Đã hoàn",
                children: `${formatVnd(payment.refundedAmount)} / ${formatVnd(payment.amount)}`,
              },
            ]}
          />
        </div>
      ) : (
        <div data-testid="payment-unavailable">
          <Alert
            type="warning"
            description="Không tải được thông tin thanh toán của đơn hàng này."
          />
        </div>
      )}

      {returns.length === 0 ? (
        <Empty description="Chưa có yêu cầu trả hàng" />
      ) : (
        returns.map((ret) => {
          const actions = returnActions(ret.status, paidOnline);
          const state = refundState(ret, payment);
          const busy = busyId === ret.id && pending;
          const codNote = ret.status === ReturnStatus.APPROVED && !paidOnline;
          return (
            <div
              key={ret.id}
              data-testid="seller-return"
              data-return-id={ret.id}
              className="space-y-3 rounded-xl border border-border-subtle bg-surface-card p-4"
            >
              <Descriptions
                title="Yêu cầu trả hàng"
                extra={
                  <OrderStatusBadge
                    kind="return"
                    status={ret.status}
                    label={ret.statusText}
                    data-testid="return-status"
                  />
                }
                column={1}
                items={[
                  { key: "reason", label: "Lý do", children: ret.reason },
                  {
                    key: "amount",
                    label: "Số tiền hoàn",
                    children: formatVnd(ret.refundAmount),
                  },
                ]}
              />
              {state.kind !== "none" && (
                <p
                  data-testid="return-refund-state"
                  className="text-sm text-text-secondary"
                >
                  {refundStateText(state)}
                </p>
              )}
              {codNote && (
                <div data-testid="return-cod-message">
                  <Alert type="info" description={COD_RETURN_MESSAGE} />
                </div>
              )}
              {actions.length > 0 && (
                <div className="flex flex-wrap gap-2">
                  {actions.includes("approve") && (
                    <Button
                      data-testid="return-approve"
                      isLoading={busy}
                      disabled={pending}
                      onClick={() => act("approve", ret)}
                    >
                      Duyệt
                    </Button>
                  )}
                  {actions.includes("refund") && (
                    <Button
                      data-testid="return-refund"
                      disabled={pending}
                      onClick={() => setConfirmId(ret.id)}
                    >
                      Hoàn tiền
                    </Button>
                  )}
                  {actions.includes("reject") && (
                    <Button
                      data-testid="return-reject"
                      variant="outline"
                      isLoading={busy}
                      disabled={pending}
                      onClick={() => act("reject", ret)}
                    >
                      Từ chối
                    </Button>
                  )}
                </div>
              )}
            </div>
          );
        })
      )}

      <Modal
        isOpen={!!confirming}
        onClose={() => !pending && setConfirmId("")}
        size="sm"
        title="Hoàn tiền cho yêu cầu này?"
        description={
          confirming
            ? `Hoàn ${formatVnd(confirming.refundAmount)} cho người mua. Số tiền thực tế được hoàn phụ thuộc vào khoản thanh toán còn lại.`
            : undefined
        }
        footer={
          <>
            <Button
              variant="outline"
              disabled={pending}
              onClick={() => setConfirmId("")}
            >
              Huỷ
            </Button>
            <Button
              data-testid="return-refund-confirm"
              isLoading={pending}
              onClick={() => confirming && act("refund", confirming)}
            >
              Xác nhận hoàn tiền
            </Button>
          </>
        }
      >
        <p className="text-sm text-text-secondary">
          Thao tác này không thể hoàn tác.
        </p>
      </Modal>
    </div>
  );
}
