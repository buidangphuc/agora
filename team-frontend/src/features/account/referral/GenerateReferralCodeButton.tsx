"use client";

import { useTransition } from "react";

import { useToast } from "@/components/ui/ToastProvider";
import { ensureReferralCodeAction } from "./actions";

/**
 * Mint the caller's referral code on demand. Wired to team-referral
 * ReferralService through the gateway via a server action.
 */
export function GenerateReferralCodeButton() {
  const [pending, start] = useTransition();
  const toast = useToast();

  function generate() {
    start(async () => {
      const res = await ensureReferralCodeAction();
      if (res.ok) {
        toast.success("✓ Đã tạo mã giới thiệu của bạn.");
      } else {
        toast.error(res.message || "Có lỗi xảy ra.");
      }
    });
  }

  return (
    <button
      type="button"
      onClick={generate}
      disabled={pending}
      className="rounded-md bg-brand px-4 py-1.5 text-sm font-medium text-white hover:opacity-90 disabled:opacity-50"
    >
      {pending ? "Đang tạo..." : "Tạo mã giới thiệu"}
    </button>
  );
}
