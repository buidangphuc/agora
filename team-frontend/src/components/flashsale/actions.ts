"use server";

import {
  type ViewFlashSaleStock,
  getFlashSaleStock,
} from "@/lib/gateway/promotion";

/**
 * Server Action polled by the LiveFlashSaleStock client component for the live
 * remaining stock of a running campaign (team-promotion GetFlashSaleStock). The
 * browser never talks to team-promotion directly (ARCHITECTURE Rule 1).
 */
export async function getFlashSaleStockAction(
  campaignId: string,
): Promise<ViewFlashSaleStock> {
  if (!campaignId) return { remaining: 0, stockCap: 0 };
  return getFlashSaleStock(campaignId);
}
