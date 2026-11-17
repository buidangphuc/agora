import { redirectGuestToLogin } from "@/features/auth/guardPath";

import { Tabs } from "@/components/ui/Tabs";
import { AnalyticsView } from "@/features/seller/AnalyticsView";
import type { ForecastItem } from "@/features/seller/ForecastCard";
import { SellerPageHeader } from "@/features/seller/SellerPageHeader";
import {
  RANGES,
  RANGE_LABEL,
  parseRange,
  rangeWindow,
} from "@/features/seller/analyticsRange";
import {
  getDemandForecast,
  getRevenueBreakdown,
  getSellerFunnel,
} from "@/lib/gateway/analytics";
import { getPrincipal, hasScope } from "@/lib/gateway/session";

export const dynamic = "force-dynamic";

export const metadata = { title: "Báo cáo doanh thu" };

const href = (range: string) => `/seller/analytics?range=${range}`;

/** Dashboard > Analysis: KPI row, funnel Progress rows, revenue Table, ?range Tabs. */
export default async function SellerAnalyticsPage(props: {
  searchParams?: Promise<{ range?: string }>;
}) {
  const searchParams = (await props.searchParams) ?? {};
  const me = getPrincipal();
  if (!me) redirectGuestToLogin("/seller/analytics", searchParams);
  if (!hasScope("listing.write")) return null;

  const range = parseRange(searchParams.range);
  const [funnel, revenue, scoped] = await Promise.allSettled([
    getSellerFunnel(me.id, { throwOnError: true }),
    getRevenueBreakdown(me.id, { throwOnError: true }),
    getRevenueBreakdown(me.id, { throwOnError: true, ...rangeWindow(range) }),
  ]);

  let forecastItems: ForecastItem[] | null = null;
  if (scoped.status === "fulfilled") {
    const picks = [...scoped.value.topSkus]
      .filter((t) => t.listingId !== "")
      .sort((a, b) => b.revenue - a.revenue)
      .slice(0, 5);
    const results = await Promise.allSettled(
      picks.map((t) => getDemandForecast(me.id, t.listingId)),
    );
    forecastItems = picks.map((t, i) => ({
      listingId: t.listingId,
      sku: t.sku,
      result: results[i],
    }));
  }

  return (
    <>
      <SellerPageHeader
        title="Báo cáo doanh thu"
        description="Phễu chuyển đổi và doanh thu theo ngày từ dữ liệu thật của shop."
      />
      <Tabs
        activeId={range}
        hrefFor={href}
        items={RANGES.map((r) => ({ id: r, label: RANGE_LABEL[r] }))}
      />
      <AnalyticsView
        funnel={funnel.status === "fulfilled" ? funnel.value : null}
        revenue={revenue.status === "fulfilled" ? revenue.value : null}
        range={range}
        retryHref={href(range)}
        forecastItems={forecastItems}
      />
    </>
  );
}
