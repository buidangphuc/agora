/**
 * Server-only gateway module for team-analytics (AnalyticsQueryService),
 * reached — like every other domain — ONLY through the gateway (ARCHITECTURE
 * Rule 1). A per-request client is built from the caller's httpOnly `session`
 * cookie, mirroring promotion.ts / notification.ts. No business logic lives
 * here: this module just forwards and maps proto → plain view types.
 */
import "server-only";

import { createPromiseClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-node";

import { AnalyticsQueryService } from "@/generated/platform/analytics/v1/analytics_connect.js";

import { authInterceptor } from "./auth.js";
import { gatewayConfig } from "./config.js";
import { getToken } from "./session.js";

function analytics() {
  const transport = createConnectTransport({
    baseUrl: gatewayConfig.gatewayUrl,
    httpVersion: "1.1",
    interceptors: [authInterceptor({ token: getToken() })],
  });
  return createPromiseClient(AnalyticsQueryService, transport);
}

export interface ViewSellerFunnel {
  impressions: number;
  views: number;
  adds: number;
  orders: number;
}

export interface ViewDayRevenue {
  day: string;
  revenue: number;
  orderCount: number;
}

export interface ViewTopSku {
  sku: string;
  listingId: string;
  revenue: number;
  unitsSold: number;
}

export interface ViewRevenueBreakdown {
  days: ViewDayRevenue[];
  topSkus: ViewTopSku[];
}

export async function getSellerFunnel(
  sellerId: string,
): Promise<ViewSellerFunnel> {
  try {
    const res = await analytics().getSellerFunnel({ sellerId });
    return {
      impressions: Number(res.impressions),
      views: Number(res.views),
      adds: Number(res.adds),
      orders: Number(res.orders),
    };
  } catch {
    return { impressions: 0, views: 0, adds: 0, orders: 0 };
  }
}

export async function getRevenueBreakdown(
  sellerId: string,
): Promise<ViewRevenueBreakdown> {
  try {
    const res = await analytics().getRevenueBreakdown({ sellerId });
    return {
      days: res.days.map((d) => ({
        day: d.day,
        revenue: Number(d.revenue),
        orderCount: Number(d.orderCount),
      })),
      topSkus: res.topSkus.map((t) => ({
        sku: t.sku,
        listingId: t.listingId,
        revenue: Number(t.revenue),
        unitsSold: Number(t.unitsSold),
      })),
    };
  } catch {
    return { days: [], topSkus: [] };
  }
}
