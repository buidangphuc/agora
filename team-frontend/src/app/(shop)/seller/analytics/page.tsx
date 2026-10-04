import { redirect } from "next/navigation";

import { Tabs } from "@/components/ui/Tabs";
import { AnalyticsView } from "@/features/seller/AnalyticsView";
import { SellerPageHeader } from "@/features/seller/SellerPageHeader";
import {
  RANGES,
  RANGE_LABEL,
  parseRange,
} from "@/features/seller/analyticsRange";
import { getRevenueBreakdown, getSellerFunnel } from "@/lib/gateway/analytics";
import { getPrincipal, hasScope } from "@/lib/gateway/session";

export const dynamic = "force-dynamic";

export const metadata = { title: "Báo cáo doanh thu | Kênh người bán" };

const href = (range: string) => `/seller/analytics?range=${range}`;

/** Dashboard > Analysis: KPI row, funnel Progress rows, revenue Table, ?range Tabs. */
export default async function SellerAnalyticsPage({
  searchParams = {},
}: { searchParams?: { range?: string } }) {
  const me = getPrincipal();
  if (!me || !hasScope("listing.write")) redirect("/login");

  const range = parseRange(searchParams.range);
  const [funnel, revenue] = await Promise.allSettled([
    getSellerFunnel(me.id, { throwOnError: true }),
    getRevenueBreakdown(me.id, { throwOnError: true }),
  ]);

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
      />
    </>
  );
}
