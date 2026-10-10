"use client";

import React, { useState } from "react";

import { Button } from "@/components/ui/Button";
import { Descriptions } from "@/components/ui/Descriptions";
import { Empty } from "@/components/ui/Empty";
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
}: {
  orderId: string;
  orderTotal: number;
  initialReturns?: ViewOrderReturn[];
  /** Offer the request action inside the empty state. */
  canRequest?: boolean;
}) {
  const [returns, addReturn] = useReturnState(initialReturns);
  const [open, setOpen] = useState(false);

  return (
    <div data-testid="return-section" className="space-y-4">
      {returns.length > 0 ? (
        returns.map((ret) => (
          <Descriptions
            key={ret.id}
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
    </div>
  );
}
