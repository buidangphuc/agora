"use client";

import { useState, useTransition } from "react";

import { useToast } from "@/components/ui/ToastProvider";
import { trackEcommerce } from "@/lib/analytics";
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
    const origin = typeof window !== "undefined" ? window.location.origin : "";
    return `${origin}/s/${shortCode}`;
  }

  function trackShare() {
    trackEcommerce("share", {
      items: [{ itemId: id }],
      properties: { method: "copy_link", content_type: "listing" },
    });
  }

  function share() {
    if (shortCode) {
      void navigator.clipboard?.writeText(shareLink());
      toast.success("✓ Đã sao chép liên kết.");
      trackShare();
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
        trackShare();
      } else {
        toast.error(res.message || "Có lỗi xảy ra.");
      }
    });
  }

  return (
    <div className="flex min-w-0 flex-wrap items-center gap-2">
      <button
        type="button"
        onClick={share}
        disabled={pending}
        className="inline-flex min-h-touch items-center rounded-xl border border-border-strong bg-surface-card px-4 text-sm font-medium text-text-primary transition duration-150 hover:bg-surface-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring disabled:opacity-50"
      >
        {pending
          ? "Đang tạo..."
          : shortCode
            ? "📋 Sao chép liên kết"
            : "🔗 Chia sẻ"}
      </button>
      {shortCode && (
        <span className="max-w-44 truncate font-mono text-xs text-text-disabled">
          {shareLink()}
        </span>
      )}
    </div>
  );
}
