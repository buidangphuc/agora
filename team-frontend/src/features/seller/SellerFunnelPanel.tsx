import { formatPrice } from "@/components/ui/format";
import type {
  ViewRevenueBreakdown,
  ViewSellerFunnel,
} from "@/lib/gateway/analytics";

/**
 * Real seller funnel + revenue numbers from team-analytics (via the gateway).
 * Presentational; thin panel above the existing mock dashboard.
 */
export function SellerFunnelPanel({
  funnel,
  revenue,
}: {
  funnel: ViewSellerFunnel;
  revenue: ViewRevenueBreakdown;
}) {
  const totalRevenue = revenue.days.reduce((s, d) => s + d.revenue, 0);
  const cells = [
    { label: "Lượt hiển thị", value: funnel.impressions.toLocaleString("vi-VN") },
    { label: "Lượt xem", value: funnel.views.toLocaleString("vi-VN") },
    { label: "Thêm giỏ", value: funnel.adds.toLocaleString("vi-VN") },
    { label: "Đơn hàng", value: funnel.orders.toLocaleString("vi-VN") },
  ];

  return (
    <div className="rounded-2xl border border-slate-100 bg-white p-6 shadow-sm">
      <div className="flex items-center justify-between">
        <h3 className="text-base font-bold text-slate-900">
          📉 Phễu chuyển đổi (dữ liệu thật)
        </h3>
        <span className="text-xs font-semibold text-slate-500">
          Doanh thu kỳ: {formatPrice(totalRevenue, "VND")}
        </span>
      </div>

      <div className="mt-4 grid grid-cols-2 gap-4 sm:grid-cols-4">
        {cells.map((c) => (
          <div key={c.label} className="rounded-xl bg-slate-50/70 p-4">
            <p className="text-xs text-slate-400">{c.label}</p>
            <p className="mt-1 text-xl font-black text-slate-900">{c.value}</p>
          </div>
        ))}
      </div>

      {revenue.topSkus.length > 0 && (
        <div className="mt-5">
          <p className="mb-2 text-xs font-bold text-slate-700">
            🏆 SKU doanh thu cao nhất
          </p>
          <ul className="divide-y divide-slate-100 text-xs">
            {revenue.topSkus.slice(0, 5).map((t) => (
              <li
                key={t.listingId || t.sku}
                className="flex items-center justify-between py-2"
              >
                <span className="truncate font-medium text-slate-800">
                  {t.sku || t.listingId}
                </span>
                <span className="shrink-0 font-bold text-slate-900">
                  {formatPrice(t.revenue, "VND")} · {t.unitsSold} sp
                </span>
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  );
}
