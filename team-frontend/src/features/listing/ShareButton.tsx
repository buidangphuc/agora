"use client";

import { useState, useTransition } from "react";

import { useToast } from "@/components/ui/ToastProvider";
import { createShareLinkAction } from "./actions";

/**
 * Create + copy a short share link for a listing. Wired to team-sharing
 * SharingService through the gateway via a server action. Tiny by design.
 */
export function ShareButton({ id }: { id: string }) {
  const [shortCode, setShortCode] = useState<string | null>(null);
  const [pending, start] = useTransition();
  const toast = useToast();

  function shareLink(): string {
    const origin =
      typeof window !== "undefined" ? window.location.origin : "";
    return `${origin}/s/${shortCode}`;
  }

  function share() {
    if (shortCode) {
      void navigator.clipboard?.writeText(shareLink());
      toast.success("✓ Đã sao chép liên kết.");
      return;
    }
    start(async () => {
      const res = await createShareLinkAction("listing", id);
      if (res.ok && res.shortCode) {
        setShortCode(res.shortCode);
        void navigator.clipboard?.writeText(
          `${window.location.origin}/s/${res.shortCode}`,
        );
        toast.success("✓ Đã tạo & sao chép liên kết chia sẻ.");
      } else {
        toast.error(res.message || "Có lỗi xảy ra.");
      }
    });
  }

  return (
    <div className="flex items-center gap-2">
      <button
        type="button"
        onClick={share}
        disabled={pending}
        className="rounded-md border border-gray-200 bg-white px-3 py-1 text-xs font-medium text-gray-700 hover:bg-gray-50 disabled:opacity-50"
      >
        {pending ? "Đang tạo..." : shortCode ? "📋 Sao chép liên kết" : "🔗 Chia sẻ"}
      </button>
      {shortCode && (
        <span className="max-w-[180px] truncate font-mono text-[11px] text-gray-400">
          {shareLink()}
        </span>
      )}
    </div>
  );
}
