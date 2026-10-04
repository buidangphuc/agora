"use client";

import React, { useState } from "react";

import { Button } from "@/components/ui/Button";
import { Descriptions } from "@/components/ui/Descriptions";
import { Empty } from "@/components/ui/Empty";
import { useToast } from "@/components/ui/ToastProvider";
import { ReturnStatus } from "@/generated/platform/order/v1/order_pb.js";
import type { ViewOrderReturn } from "@/lib/gateway/orders";
import { OrderStatusBadge } from "./OrderStatusBadge";
import { ReturnRequestModal } from "./ReturnRequestModal";
import { useReturnState } from "./ReturnState";
import { mockRefundAction } from "./actions";
import { usePendingAction } from "./usePendingAction";

/**
 * The returns tab body: the existing return as Descriptions plus its status,
 * or Empty. The request form lives in `ReturnRequestModal`, opened from the
 * order header (or from the Empty action when `canRequest`).
 */
export function ReturnRequestSection({
  orderId,
  orderTotal,
  initialReturn = null,
  canRequest = false,
}: {
  orderId: string;
  orderTotal: number;
  initialReturn?: ViewOrderReturn | null;
  /** Offer the request action inside the empty state. */
  canRequest?: boolean;
}) {
  const toast = useToast();
  const [ret, setRet] = useReturnState(initialReturn);
  const [open, setOpen] = useState(false);
  const [pending, run] = usePendingAction();

  function refund() {
    if (pending || !ret) return;
    void run(async () => {
      const res = await mockRefundAction(ret.id, orderId, ret.refundAmount);
      if (res.ok && res.data) {
        setRet(res.data);
        toast.success("Hoàn tiền (mô phỏng) thành công.");
      } else if (!res.ok) {
        toast.error(res.error);
      }
    });
  }

  const canRefund =
    !!ret &&
    ret.status !== ReturnStatus.REFUNDED &&
    ret.status !== ReturnStatus.REJECTED;

  return (
    <div data-testid="return-section" className="space-y-4">
      {ret ? (
        <>
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
                children: `${ret.refundAmount.toLocaleString("vi-VN")}₫`,
              },
            ]}
          />
          {canRefund && (
            <Button
              variant="secondary"
              data-testid="return-refund"
              isLoading={pending}
              onClick={refund}
            >
              Hoàn tiền (mô phỏng)
            </Button>
          )}
        </>
      ) : (
        <Empty
          description="Chưa có yêu cầu trả hàng"
          action={
            canRequest ? (
              <Button variant="outline" onClick={() => setOpen(true)}>
                Yêu cầu trả hàng
              </Button>
            ) : undefined
          }
        />
      )}

      <ReturnRequestModal
        isOpen={open}
        onClose={() => setOpen(false)}
        orderId={orderId}
        orderTotal={orderTotal}
        onCreated={setRet}
      />
    </div>
  );
}
