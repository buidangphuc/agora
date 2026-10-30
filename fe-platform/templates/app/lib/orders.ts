import type { Order } from "@fe-platform/api-client";
import { formatMinor } from "./money";
import { orderStatusLabel } from "./order-status";

export type OrderItemView = {
  title: string;
  variantName: string;
  quantity: number;
  unitPrice: string;
  imageUrl: string;
};

export type OrderView = {
  id: string;
  status: string;
  total: string;
  shippingFee: string;
  placedOn: string;
  trackingNumber: string;
  recipient: string;
  address: string;
  items: OrderItemView[];
};

export function toOrderView(o: Order): OrderView {
  const a = o.shippingAddress;
  return {
    id: o.id,
    status: orderStatusLabel(o.status),
    total: formatMinor(o.totalAmount, o.currency),
    shippingFee: formatMinor(o.shippingFee, o.currency),
    placedOn: o.createdAt
      ? new Intl.DateTimeFormat("en", { dateStyle: "medium" }).format(
          o.createdAt.toDate(),
        )
      : "",
    trackingNumber: o.trackingNumber,
    recipient: a?.recipientName ?? "",
    address: a
      ? [a.street, a.ward, a.district, a.city].filter(Boolean).join(", ")
      : "",
    items: o.items.map((i) => ({
      title: i.title,
      variantName: i.variantName,
      quantity: i.quantity,
      unitPrice: formatMinor(i.unitPrice, o.currency),
      imageUrl: i.imageUrl,
    })),
  };
}
