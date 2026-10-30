import { Tag } from "@/components/ui/Tag";
import type { ViewOrder } from "@/lib/gateway/orders";
import { isToShip } from "./status";

/** A to-ship order whose ship-by deadline is strictly in the past. */
export function isOverdue(
  o: Pick<ViewOrder, "status" | "paymentMethod" | "shipBy">,
  now: number = Date.now(),
): boolean {
  if (!o.shipBy || !isToShip(o)) return false;
  const t = Date.parse(o.shipBy);
  return !Number.isNaN(t) && t < now;
}

function formatShipBy(iso: string): string {
  const t = Date.parse(iso);
  if (Number.isNaN(t)) return "-";
  return new Date(t).toLocaleString("vi-VN", { timeZone: "Asia/Ho_Chi_Minh" });
}

/** Ship-by deadline text plus an overdue tag; "-" when there is no deadline. */
export function ShipByLabel({ order }: { order: ViewOrder }) {
  return (
    <span className="inline-flex items-center gap-1">
      <span>{order.shipBy ? formatShipBy(order.shipBy) : "-"}</span>
      {isOverdue(order) && <Tag color="danger">Quá hạn</Tag>}
    </span>
  );
}
