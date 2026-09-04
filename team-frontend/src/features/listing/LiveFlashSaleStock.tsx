"use client";

import { useEffect, useState } from "react";

import { getFlashSaleStockAction } from "@/components/flashsale/actions";
import { formatPrice } from "@/components/ui/format";

/** Plain (client-safe) shape of an active flash-sale campaign, resolved
 *  server-side on the PDP and passed in as a prop. */
export interface FlashSaleCampaignView {
  id: string;
  salePrice: number;
  stockCap: number;
  stockSold: number;
  remaining: number;
}

const POLL_MS = 5000;

/**
 * Live flash-sale banner for the product detail page. When the listing has an
 * active campaign it shows the sale price and a remaining-stock meter, polling
 * team-promotion (via a server action → gateway) for the live remaining count.
 * Renders nothing when the listing is not on flash sale.
 */
export function LiveFlashSaleStock({
  listingId,
  campaign,
  currency = "VND",
}: {
  listingId: string;
  campaign?: FlashSaleCampaignView | null;
  currency?: string;
}) {
  const [remaining, setRemaining] = useState(campaign?.remaining ?? 0);
  const cap = campaign?.stockCap ?? 0;
  const campaignId = campaign?.id;

  useEffect(() => {
    if (!campaignId) return;
    let cancelled = false;

    async function poll() {
      try {
        const res = await getFlashSaleStockAction(campaignId as string);
        if (!cancelled) setRemaining(res.remaining);
      } catch {
        // best-effort live update; keep last known value
      }
    }

    poll();
    const timer = setInterval(poll, POLL_MS);
    return () => {
      cancelled = true;
      clearInterval(timer);
    };
  }, [campaignId]);

  if (!campaign) return null;

  const sold = Math.max(0, cap - remaining);
  const percentage =
    cap > 0 ? Math.min(100, Math.round((sold / cap) * 100)) : 0;
  const soldOut = remaining <= 0;

  return (
    <div
      data-listing-id={listingId}
      className="bg-gradient-to-r from-amber-50 to-orange-50 border border-orange-200 rounded-xl p-3.5 my-3 shadow-xs"
    >
      <div className="flex items-center justify-between text-xs font-bold mb-1.5">
        <div className="flex items-center gap-1.5 text-orange-700">
          <span className="animate-bounce text-sm">🔥</span>
          <span className="uppercase tracking-wider">FLASH SALE</span>
        </div>
        <span className="text-brand font-extrabold text-base">
          {formatPrice(campaign.salePrice, currency)}
        </span>
      </div>

      {/* Remaining-stock meter (e2e target). */}
      <div
        data-testid="flash-sale-stock"
        data-remaining={remaining}
        data-stock-cap={cap}
        className="relative w-full h-4 bg-orange-200/80 rounded-full overflow-hidden shadow-inner"
      >
        <div
          className="h-full bg-gradient-to-r from-amber-500 via-orange-500 to-red-600 rounded-full transition-all duration-700 ease-out"
          style={{ width: `${percentage}%` }}
        />
        <div className="absolute inset-0 flex items-center justify-center text-[10px] font-black text-white drop-shadow-[0_1px_2px_rgba(0,0,0,0.6)]">
          {soldOut ? "ĐÃ BÁN HẾT" : `ĐÃ BÁN ${sold} / ${cap}`}
        </div>
      </div>

      <div className="text-[10px] text-orange-600/90 mt-1.5 flex items-center justify-between">
        <span>⚡ Số lượng có hạn — cập nhật trực tiếp</span>
        <span className="font-semibold text-red-600">
          Còn {remaining} sản phẩm
        </span>
      </div>
    </div>
  );
}
