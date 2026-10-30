"use client";

import React, { useState } from "react";

import { Button } from "@/components/ui/Button";
import { Descriptions } from "@/components/ui/Descriptions";
import { Empty } from "@/components/ui/Empty";
import { EscalateDisputeModal } from "@/features/dispute/EscalateDisputeModal";
import { OrderDisputeList } from "@/features/dispute/OrderDisputeList";
import { isOpenDispute } from "@/features/dispute/allowedActions";
import type { ViewDispute } from "@/lib/gateway/disputes";
import type { ViewOrderReturn } from "@/lib/gateway/orders";
import { OrderStatusBadge } from "./OrderStatusBadge";
import { ReturnRequestModal } from "./ReturnRequestModal";
import { useReturnState } from "./ReturnState";

/**
 * The returns tab body: each of the order's returns as Descriptions plus its
 * status, or Empty. There is no refund control here: the seller refunds, and
 * the buyer only follows the status. The request form lives in
 * `ReturnRequestModal`, opened from the order header (or from the Empty
 * action when `canRequest`).
 */
export function ReturnRequestSection({
  orderId,
  orderTotal,
  initialReturns = [],
  canRequest = false,
  sellerId = "",
  disputes: initialDisputes = [],
  disputesFailed = false,
}: {
  orderId: string;
  orderTotal: number;
  initialReturns?: ViewOrderReturn[];
  /** Offer the request action inside the empty state. */
  canRequest?: boolean;
  sellerId?: string;
  disputes?: ViewDispute[];
  disputesFailed?: boolean;
}) {
  const [returns, addReturn] = useReturnState(initialReturns);
  const [open, setOpen] = useState(false);
  const [disputes, setDisputes] = useState<ViewDispute[]>(initialDisputes);
  const [escalating, setEscalating] = useState<string | null>(null);

  const canEscalateReturn = (ret: ViewOrderReturn) =>
    ret.canEscalate &&
    !disputes.some((d) => d.returnId === ret.id && isOpenDispute(d));

  return (
    <div data-testid="return-section" className="flex flex-col gap-4">
      {returns.length > 0 ? (
        returns.map((ret) => (
          <React.Fragment key={ret.id}>
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
            {canEscalateReturn(ret) && (
              <Button
                variant="outline"
                fullWidth="mobile"
                onClick={() => setEscalating(ret.id)}
              >
                Khiếu nại lên Agora
              </Button>
            )}
          </React.Fragment>
        ))
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
        onCreated={addReturn}
      />

      <OrderDisputeList disputes={disputes} failed={disputesFailed} />

      {escalating !== null && (
        <EscalateDisputeModal
          isOpen
          onClose={() => setEscalating(null)}
          orderId={orderId}
          sellerId={sellerId}
          returnId={escalating}
          onCreated={(d) => setDisputes((cur) => [d, ...cur])}
        />
      )}
    </div>
  );
}
