import { OrderStatus } from "@/generated/platform/order/v1/order_pb.js";
import type { ListingPage } from "@/lib/gateway/listings";
import type { ViewOrder } from "@/lib/gateway/orders";

export const LOW_STOCK_THRESHOLD = 5;

export interface Kpi {
  key: string;
  title: string;
  value: number;
}

/**
 * Workplace KPIs, derived only from gateway data. A KPI whose source call
 * failed (`null`) is omitted instead of shown as a placeholder zero.
 */
export function deriveWorkplaceKpis(sources: {
  listings: ListingPage | null;
  orders: ViewOrder[] | null;
}): Kpi[] {
  const kpis: Kpi[] = [];
  const { listings, orders } = sources;
  if (listings) {
    kpis.push({ key: "total", title: "Tổng sản phẩm", value: listings.total });
    kpis.push({
      key: "published",
      title: "Đang bán",
      value: listings.items.filter((l) => l.status === "published").length,
    });
    kpis.push({
      key: "low-stock",
      title: `Sắp hết hàng (≤ ${LOW_STOCK_THRESHOLD})`,
      value: listings.items.filter((l) => l.stock <= LOW_STOCK_THRESHOLD)
        .length,
    });
  }
  if (orders) {
    kpis.push({
      key: "open-orders",
      title: "Đơn chờ xử lý",
      value: orders.filter(
        (o) =>
          o.status === OrderStatus.PENDING || o.status === OrderStatus.PAID,
      ).length,
    });
  }
  return kpis;
}
