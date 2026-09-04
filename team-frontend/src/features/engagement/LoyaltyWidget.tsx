"use client";

import { useState, useTransition } from "react";

import { useToast } from "@/components/ui/ToastProvider";
import type { ViewLoyalty } from "@/lib/gateway/engagement";
import { checkInAction } from "./actions";

/**
 * Thin daily check-in ("Điểm danh") widget: shows the streak + coin balance and
 * a check-in button wired to team-engagement through the gateway. Styling to be
 * revamped later.
 */
export function LoyaltyWidget({ initial }: { initial: ViewLoyalty }) {
  const [streak, setStreak] = useState(initial.streak);
  const [coins, setCoins] = useState(initial.coinBalance);
  const [earned, setEarned] = useState<number | null>(null);
  const [pending, start] = useTransition();
  const toast = useToast();

  function checkIn() {
    start(async () => {
      const res = await checkInAction();
      if (res.ok && res.result) {
        setStreak(res.result.streak);
        setCoins(res.result.coinBalance);
        setEarned(res.result.coinsEarned);
        toast.success(`✓ Điểm danh thành công! +${res.result.coinsEarned} xu`);
      } else {
        toast.error(res.message || "Có lỗi xảy ra.");
      }
    });
  }

  return (
    <section className="flex items-center justify-between gap-3 rounded-xs border border-yellow-200 bg-yellow-50 px-4 py-3 shadow-shopee">
      <div className="flex items-center gap-3 text-sm">
        <span className="text-2xl">🪙</span>
        <div>
          <p className="font-bold text-yellow-800">Điểm danh nhận xu</p>
          <p className="text-xs text-yellow-700">
            Chuỗi ngày: <strong>{streak}</strong> · Số dư:{" "}
            <strong>{coins.toLocaleString("vi-VN")} xu</strong>
            {earned !== null && (
              <span className="ml-1 text-emerald-700">(+{earned})</span>
            )}
          </p>
        </div>
      </div>
      <button
        type="button"
        onClick={checkIn}
        disabled={pending}
        className="rounded-md bg-brand px-4 py-1.5 text-xs font-semibold text-white shadow-xs transition hover:opacity-90 disabled:opacity-50"
      >
        {pending ? "Đang điểm danh..." : "Điểm danh"}
      </button>
    </section>
  );
}
