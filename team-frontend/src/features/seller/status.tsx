import { Tag, type TagColor } from "@/components/ui/Tag";
import { OrderStatus } from "@/generated/platform/order/v1/order_pb.js";

const LISTING: Record<string, { color: TagColor; text: string }> = {
  published: { color: "success", text: "Đang bán" },
  draft: { color: "neutral", text: "Bản nháp" },
  rejected: { color: "danger", text: "Bị từ chối" },
};

export function ListingStatusTag({ status }: { status: string }) {
  const entry = LISTING[status] ?? { color: "neutral", text: "Chưa xác định" };
  return <Tag color={entry.color}>{entry.text}</Tag>;
}

const ORDER: Partial<Record<OrderStatus, TagColor>> = {
  [OrderStatus.PENDING]: "warning",
  [OrderStatus.PAID]: "info",
  [OrderStatus.SHIPPED]: "primary",
  [OrderStatus.COMPLETED]: "success",
  [OrderStatus.CANCELLED]: "neutral",
};

export function OrderStatusTag({
  status,
  text,
}: { status: OrderStatus; text: string }) {
  return <Tag color={ORDER[status] ?? "neutral"}>{text}</Tag>;
}

/** An order the seller can still hand over to the carrier. */
export function isShippable(status: OrderStatus): boolean {
  return status === OrderStatus.PENDING || status === OrderStatus.PAID;
}
