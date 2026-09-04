"use server";

import { ConnectError } from "@connectrpc/connect";

import type { ViewListing } from "@/lib/gateway/listings";
import {
  RecommendationContext,
  getRecommendations,
} from "@/lib/gateway/recommendations";

export interface RecommendationsResult {
  ok: boolean;
  message: string;
  items: ViewListing[];
}

/**
 * Server Action: fetch the "Gợi ý cho bạn" cards through the gateway. Always
 * resolves (never throws to the client): on failure it returns an empty list so
 * the row can be hidden rather than breaking the page.
 */
export async function getRecommendationsAction(
  opts: {
    seedListingId?: string;
    context?: RecommendationContext;
    limit?: number;
  } = {},
): Promise<RecommendationsResult> {
  try {
    const items = await getRecommendations(opts);
    return { ok: true, message: "", items };
  } catch (err) {
    const msg =
      err instanceof ConnectError
        ? err.message
        : "Không tải được gợi ý sản phẩm.";
    return { ok: false, message: msg, items: [] };
  }
}
