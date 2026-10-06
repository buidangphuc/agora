import React from "react";

import { Tag, type TagColor } from "@/components/ui/Tag";
import {
  OrderStatus,
  ReturnStatus,
} from "@/generated/platform/order/v1/order_pb.js";

const orderTone: Partial<Record<OrderStatus, TagColor>> = {
  [OrderStatus.PENDING]: "warning",
  [OrderStatus.PAID]: "info",
  [OrderStatus.SHIPPED]: "info",
  [OrderStatus.COMPLETED]: "success",
  [OrderStatus.CANCELLED]: "neutral",
};

const returnTone: Partial<Record<ReturnStatus, TagColor>> = {
  [ReturnStatus.PENDING]: "warning",
  [ReturnStatus.APPROVED]: "info",
  [ReturnStatus.REJECTED]: "danger",
  [ReturnStatus.REFUNDED]: "success",
};

export type OrderStatusBadgeProps = (
  | { kind?: "order"; status: OrderStatus }
  | { kind: "return"; status: ReturnStatus }
) & {
  /** The status text from the view model (`statusText`). */
  label: string;
  className?: string;
  "data-testid"?: string;
};

/** Tone of a status; anything unrecognised is neutral. */
export function orderStatusTone(
  props: Pick<OrderStatusBadgeProps, "kind" | "status">,
): TagColor {
  const tone =
    props.kind === "return"
      ? returnTone[props.status as ReturnStatus]
      : orderTone[props.status as OrderStatus];
  return tone ?? "neutral";
}

/**
 * The one place an order or return status is presented: a `Tag` in a semantic
 * tone (never the brand colour). Server-compatible.
 */
export function OrderStatusBadge({
  label,
  className,
  "data-testid": testId,
  ...status
}: OrderStatusBadgeProps) {
  return (
    <Tag
      color={orderStatusTone(status)}
      className={className}
      data-testid={testId}
    >
      {label}
    </Tag>
  );
}
