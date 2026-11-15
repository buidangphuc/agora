import { OrderStatus } from "@/generated/platform/order/v1/order_pb.js";

export interface Kpi {
  key: string;
  title: string;
  value: number;
  note?: string;
  tone?: "danger";
}

/**
 * Workplace KPIs, derived from server-side counts. A KPI whose source failed
 * (`null`) is omitted instead of shown as a placeholder zero.
 */
export function deriveWorkplaceKpis(sources: {
  listingsTotal: number | null;
  publishedTotal: number | null;
  statusCounts: Partial<Record<OrderStatus, number>> | null;
}): Kpi[] {
  const kpis: Kpi[] = [];
  const { listingsTotal, publishedTotal, statusCounts } = sources;
  if (listingsTotal !== null) {
    kpis.push({ key: "total", title: "Tổng sản phẩm", value: listingsTotal });
  }
  if (publishedTotal !== null) {
    kpis.push({ key: "published", title: "Đang bán", value: publishedTotal });
  }
  if (statusCounts) {
    kpis.push({
      key: "to-ship",
      title: "Cần gửi",
      value:
        (statusCounts[OrderStatus.PENDING] ?? 0) +
        (statusCounts[OrderStatus.PAID] ?? 0),
    });
  }
  return kpis;
}
