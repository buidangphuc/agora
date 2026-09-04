"use client";

import { useState, useTransition } from "react";

import { useToast } from "@/components/ui/ToastProvider";
import type { ViewAdCampaign } from "@/lib/gateway/promotion";
import { createAdCampaignAction } from "./actions";

interface SellerListingOption {
  id: string;
  title: string;
}

/**
 * Thin sponsored-ad campaign form: pick a listing, set budget + bid, and launch a
 * campaign. Wired to team-promotion SponsoredService through the gateway via a
 * server action. Styling to be revamped later.
 */
export function AdCampaignForm({
  listings,
}: {
  listings: SellerListingOption[];
}) {
  const [listingId, setListingId] = useState(listings[0]?.id ?? "");
  const [budget, setBudget] = useState("");
  const [bid, setBid] = useState("");
  const [campaigns, setCampaigns] = useState<ViewAdCampaign[]>([]);
  const [pending, start] = useTransition();
  const toast = useToast();

  function submit() {
    start(async () => {
      const res = await createAdCampaignAction(
        listingId,
        Number(budget) || 0,
        Number(bid) || 0,
      );
      if (res.ok && res.campaign) {
        setCampaigns((prev) => [res.campaign as ViewAdCampaign, ...prev]);
        setBudget("");
        setBid("");
        toast.success("✓ Đã tạo chiến dịch quảng cáo.");
      } else {
        toast.error(res.message || "Có lỗi xảy ra.");
      }
    });
  }

  return (
    <div className="space-y-4">
      <div className="rounded-xs border border-gray-200 bg-white p-4 shadow-2xs space-y-3">
        <h2 className="text-sm font-bold text-gray-800">Tạo chiến dịch mới</h2>
        <select
          value={listingId}
          onChange={(e) => setListingId(e.target.value)}
          className="w-full rounded-xs border border-gray-300 px-3 py-2 text-xs outline-none focus:border-brand"
        >
          {listings.length === 0 ? (
            <option value="">Shop chưa có sản phẩm</option>
          ) : (
            listings.map((l) => (
              <option key={l.id} value={l.id}>
                {l.title}
              </option>
            ))
          )}
        </select>
        <input
          value={budget}
          onChange={(e) => setBudget(e.target.value)}
          type="number"
          min={0}
          placeholder="Ngân sách (VND)"
          className="w-full rounded-xs border border-gray-300 px-3 py-2 text-xs outline-none focus:border-brand"
        />
        <input
          value={bid}
          onChange={(e) => setBid(e.target.value)}
          type="number"
          min={0}
          placeholder="Giá thầu / lượt (VND)"
          className="w-full rounded-xs border border-gray-300 px-3 py-2 text-xs outline-none focus:border-brand"
        />
        <button
          type="button"
          onClick={submit}
          disabled={pending || !listingId}
          className="rounded-xs bg-brand px-4 py-2 text-xs font-bold text-white shadow-xs hover:bg-brand-dark disabled:opacity-50"
        >
          {pending ? "Đang tạo..." : "Chạy quảng cáo"}
        </button>
      </div>

      {campaigns.length > 0 && (
        <div className="rounded-xs border border-gray-200 bg-white p-4 shadow-2xs">
          <h2 className="mb-3 text-sm font-bold text-gray-800">
            Chiến dịch vừa tạo
          </h2>
          <ul className="divide-y divide-gray-100">
            {campaigns.map((c) => (
              <li
                key={c.id}
                className="flex items-center justify-between gap-3 py-2.5 text-xs"
              >
                <div>
                  <p className="font-medium text-gray-800">
                    #{c.id.slice(0, 8)}
                  </p>
                  <p className="text-gray-400">
                    Ngân sách: {c.budget.toLocaleString("vi-VN")} · Thầu:{" "}
                    {c.bid.toLocaleString("vi-VN")}
                  </p>
                </div>
                <span className="rounded bg-emerald-50 px-2 py-0.5 font-semibold text-emerald-700">
                  {c.statusText}
                </span>
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  );
}
