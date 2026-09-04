"use client";

import { useState, useTransition } from "react";

import { useToast } from "@/components/ui/ToastProvider";
import { redeemReferralAction } from "./actions";

/**
 * Redeem someone else's referral code. Wired to team-referral ReferralService
 * through the gateway via a server action. Thin — styling to be revamped later.
 */
export function RedeemReferralForm() {
  const [code, setCode] = useState("");
  const [pending, start] = useTransition();
  const toast = useToast();

  function submit() {
    start(async () => {
      const res = await redeemReferralAction(code);
      if (res.ok) {
        setCode("");
        toast.success("✓ Đã nhập mã giới thiệu.");
      } else {
        toast.error(res.message || "Có lỗi xảy ra.");
      }
    });
  }

  return (
    <div className="flex gap-2">
      <input
        type="text"
        value={code}
        onChange={(e) => setCode(e.target.value)}
        placeholder="Nhập mã của bạn bè"
        className="flex-1 rounded-md border border-gray-200 px-3 py-1.5 text-sm focus:border-brand focus:outline-none"
      />
      <button
        type="button"
        onClick={submit}
        disabled={pending || !code.trim()}
        className="rounded-md bg-brand px-4 py-1.5 text-sm font-medium text-white hover:opacity-90 disabled:opacity-50"
      >
        {pending ? "Đang nhập..." : "Nhập mã"}
      </button>
    </div>
  );
}
