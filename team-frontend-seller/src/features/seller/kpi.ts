import type { ListingPage } from "@/lib/gateway/listings";
import type { ViewOrder } from "@/lib/gateway/orders";
import { isOverdue } from "./shipBy";
import { isToShip } from "./status";

export const LOW_STOCK_THRESHOLD = 5;

export interface Kpi {
  key: string;
  title: string;
  value: number;
  note?: string;
  tone?: "danger";
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
    const toShip = orders.filter(isToShip);
    const overdue = toShip.filter((o) => isOverdue(o)).length;
    kpis.push({
      key: "to-ship",
      title: "Cần gửi",
      value: toShip.length,
      ...(overdue > 0 && {
        note: `Quá hạn: ${overdue}`,
        tone: "danger" as const,
      }),
    });
  }
  return kpis;
}
