import { redirect } from "next/navigation";

import { SellerAnalyticsMock } from "@/features/seller/SellerAnalyticsMock";
import { SellerFunnelPanel } from "@/features/seller/SellerFunnelPanel";
import {
  getRevenueBreakdown,
  getSellerFunnel,
} from "@/lib/gateway/analytics";
import { getPrincipal, hasScope } from "@/lib/gateway/session";

export const dynamic = "force-dynamic";

export default async function SellerAnalyticsPage() {
  const me = getPrincipal();
  if (!me || !hasScope("listing.write")) redirect("/login");

  const [funnel, revenue] = await Promise.all([
    getSellerFunnel(me.id),
    getRevenueBreakdown(me.id),
  ]);

  return (
    <div className="space-y-6">
      {/* Real numbers from team-analytics (gateway-only). */}
      <SellerFunnelPanel funnel={funnel} revenue={revenue} />

      {/* Existing mock dashboard — to be revamped later. */}
      <SellerAnalyticsMock />
    </div>
  );
}
